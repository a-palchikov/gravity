/*
Copyright 2018 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package process

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/gravitational/gravity/lib/blob/fs"
	"github.com/gravitational/gravity/lib/constants"
	"github.com/gravitational/gravity/lib/defaults"
	"github.com/gravitational/gravity/lib/loc"
	"github.com/gravitational/gravity/lib/pack"
	"github.com/gravitational/gravity/lib/pack/localpack"
	"github.com/gravitational/gravity/lib/storage"
	"github.com/gravitational/gravity/lib/storage/keyval"
	"github.com/gravitational/gravity/lib/utils"

	telecfg "github.com/gravitational/teleport/lib/config"
	"github.com/gravitational/teleport/lib/service"
	teleservices "github.com/gravitational/teleport/lib/services"
	teleutils "github.com/gravitational/teleport/lib/utils"
	"github.com/gravitational/trace"

	"github.com/coreos/go-semver/semver"
	"github.com/sirupsen/logrus"
	"gopkg.in/check.v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ProcessSuite struct{}

var _ = check.Suite(&ProcessSuite{})

func (s *ProcessSuite) TestAuthGatewayConfigReload(c *check.C) {
	// Initialize process with some default configuration.
	teleportConfig := telecfg.MakeSampleFileConfig()
	teleportConfig.DataDir = c.MkDir()
	teleportConfig.Proxy.CertFile = ""
	teleportConfig.Proxy.KeyFile = ""
	backend, err := keyval.NewBolt(keyval.BoltConfig{
		Path: filepath.Join(c.MkDir(), "test.db"),
	})
	c.Assert(err, check.IsNil)
	process := &Process{
		FieldLogger:       logrus.WithField(trace.Component, "process"),
		backend:           backend,
		tcfg:              *teleportConfig,
		authGatewayConfig: storage.DefaultAuthGateway(),
	}
	serviceConfig, err := process.buildTeleportConfig(process.authGatewayConfig)
	c.Assert(err, check.IsNil)
	process.TeleportProcess = &service.TeleportProcess{
		Supervisor: service.NewSupervisor("test"),
		Config:     serviceConfig,
	}

	// Update auth gateway setting that should trigger reload.
	err = process.reloadAuthGatewayConfig(storage.NewAuthGateway(
		storage.AuthGatewaySpecV1{
			ConnectionLimits: &storage.ConnectionLimits{
				MaxConnections: utils.Int64Ptr(50),
			},
		}))
	c.Assert(err, check.IsNil)
	// Make sure reload event was broadcast.
	ch := make(chan service.Event)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	process.WaitForEvent(ctx, service.TeleportReloadEvent, ch)
	select {
	case <-ch:
	case <-ctx.Done():
		c.Fatal("didn't receive reload event")
	}

	// Now update principals.
	err = process.reloadAuthGatewayConfig(storage.NewAuthGateway(
		storage.AuthGatewaySpecV1{
			PublicAddr: &[]string{"example.com"},
		}))
	c.Assert(err, check.IsNil)
	// Make sure process config is updated.
	config := process.TeleportProcess.Config
	comparePrincipals(c, config.Auth.PublicAddrs, []string{"example.com"})
	comparePrincipals(c, config.Proxy.SSHPublicAddrs, []string{"example.com"})
	comparePrincipals(c, config.Proxy.PublicAddrs, []string{"example.com"})
	comparePrincipals(c, config.Proxy.Kube.PublicAddrs, []string{"example.com"})
}

func comparePrincipals(c *check.C, addrs []teleutils.NetAddr, principals []string) {
	var hosts []string
	for _, addr := range addrs {
		hosts = append(hosts, addr.Host())
	}
	c.Assert(hosts, check.DeepEquals, principals)
}

func (s *ProcessSuite) TestClusterServices(c *check.C) {
	p := Process{
		context: context.TODO(),
	}

	// initially no services are running
	c.Assert(p.clusterServicesRunning(), check.Equals, false)

	service1Launched := make(chan bool)
	service1Done := make(chan bool)
	service1 := func(ctx context.Context) {
		close(service1Launched)
		defer close(service1Done)
		<-ctx.Done()
	}

	service2Launched := make(chan bool)
	service2Done := make(chan bool)
	service2 := func(ctx context.Context) {
		close(service2Launched)
		defer close(service2Done)
		<-ctx.Done()
	}

	// launch services
	p.clusterServices = []clusterService{service1, service2}
	err := p.startClusterServices()
	c.Assert(err, check.IsNil)
	for i, ch := range []chan bool{service1Launched, service2Launched} {
		select {
		case <-ch:
		case <-time.After(time.Second):
			c.Fatalf("service%v wasn't launched", i+1)
		}
	}
	c.Assert(p.clusterServicesRunning(), check.Equals, true)

	// should not attempt to launch again
	err = p.startClusterServices()
	c.Assert(err, check.NotNil)
	c.Assert(p.clusterServicesRunning(), check.Equals, true)

	// stop services
	err = p.stopClusterServices()
	c.Assert(err, check.IsNil)
	for i, ch := range []chan bool{service1Done, service2Done} {
		select {
		case <-ch:
		case <-time.After(time.Second):
			c.Fatalf("service%v wasn't stopped", i+1)
		}
	}
	c.Assert(p.clusterServicesRunning(), check.Equals, false)

	// should not attempt to stop again
	err = p.stopClusterServices()
	c.Assert(err, check.NotNil)
	c.Assert(p.clusterServicesRunning(), check.Equals, false)
}

func (s *ProcessSuite) TestReverseTunnelsFromTrustedClusters(c *check.C) {
	var testCases = []struct {
		clusters []teleservices.TrustedCluster
		tunnels  []telecfg.ReverseTunnel
		comment  string
	}{
		{
			clusters: nil,
			tunnels:  nil,
			comment:  "does nothing",
		},
		{
			clusters: []teleservices.TrustedCluster{
				storage.NewTrustedCluster("cluster1", storage.TrustedClusterSpecV2{
					Enabled:              false,
					ReverseTunnelAddress: "cluster1:3024",
					ProxyAddress:         "cluster1:443",
					Token:                "secret",
					Roles:                []string{constants.RoleAdmin},
				}),
			},
			tunnels: nil,
			comment: "ignores disabled clusters",
		},
		{
			clusters: []teleservices.TrustedCluster{
				storage.NewTrustedCluster("cluster1", storage.TrustedClusterSpecV2{
					Enabled:              true,
					ReverseTunnelAddress: "cluster1:3024",
					ProxyAddress:         "cluster1:443",
					Token:                "secret",
					Roles:                []string{constants.RoleAdmin},
				}),
				storage.NewTrustedCluster("cluster2", storage.TrustedClusterSpecV2{
					Enabled:              true,
					ReverseTunnelAddress: "cluster2:3024",
					ProxyAddress:         "cluster2:443",
					Token:                "secret",
					Roles:                []string{constants.RoleAdmin},
					Wizard:               true,
				}),
			},
			tunnels: []telecfg.ReverseTunnel{
				{
					DomainName: "cluster1",
					Addresses:  []string{"cluster1:3024"},
				},
				{
					DomainName: "cluster2",
					Addresses:  []string{"cluster2:3024"},
				},
			},
			comment: "considers all remote trusted clusters",
		},
	}
	for _, testCase := range testCases {
		backend, err := keyval.NewBolt(keyval.BoltConfig{
			Path: filepath.Join(c.MkDir(), "test.db"),
		})
		c.Assert(err, check.IsNil)
		for _, cluster := range testCase.clusters {
			_, err := backend.UpsertTrustedCluster(cluster)
			c.Assert(err, check.IsNil)
		}
		tunnels, err := reverseTunnelsFromTrustedClusters(backend)
		c.Assert(err, check.IsNil)
		c.Assert(tunnels, check.DeepEquals, testCase.tunnels, check.Commentf(testCase.comment))
	}
}

func (s *importerSuite) TestCorrectlySelectsNewTeleportConfig(c *check.C) {
	// setup
	s.addTeleportPackages(c,
		"example.com/teleport-master-config:0.0.12345",
		"example.com/teleport-master-config:1.0.0",
		"example.com/teleport-master-config:1.0.1",
	)
	teleportVersion := semver.New("1.0.1")
	i := &importer{
		backend:  s.backend,
		packages: s.pack,
	}
	// exercise
	teleportConfig, err := i.findLatestTeleportConfigPackage("example.com", *teleportVersion)
	// verify
	c.Assert(err, check.IsNil)
	c.Assert(teleportConfig, check.DeepEquals, &loc.Locator{
		Repository: "example.com",
		Name:       "teleport-master-config",
		Version:    "1.0.1",
	})
}

func (s *importerSuite) TestCorrectlySelectsLegacyTeleportConfig(c *check.C) {
	// setup
	s.addTeleportPackages(c,
		"example.com/teleport-master-config:0.0.12345",
	)
	teleportVersion := semver.New("1.0.1")
	i := &importer{
		backend:  s.backend,
		packages: s.pack,
	}
	// exercise
	teleportConfig, err := i.findLatestTeleportConfigPackage("example.com", *teleportVersion)
	// verify
	c.Assert(err, check.IsNil)
	c.Assert(teleportConfig, check.DeepEquals, &loc.Locator{
		Repository: "example.com",
		Name:       "teleport-master-config",
		Version:    "0.0.12345",
	})
}

func (s *importerSuite) SetUpTest(c *check.C) {
	s.dir = c.MkDir()

	var err error
	s.backend, err = keyval.NewBolt(keyval.BoltConfig{
		Path: filepath.Join(s.dir, "bolt.db"),
	})
	c.Assert(err, check.IsNil)

	objects, err := fs.New(s.dir)
	c.Assert(err, check.IsNil)

	s.pack, err = localpack.New(localpack.Config{
		Backend:     s.backend,
		UnpackedDir: filepath.Join(s.dir, defaults.UnpackedDir),
		Objects:     objects,
	})
	c.Assert(err, check.IsNil)
}

func (s *importerSuite) addTeleportPackages(c *check.C, packages ...string) {
	err := s.pack.UpsertRepository("example.com", time.Time{})
	c.Assert(err, check.IsNil)
	for i, pkg := range packages {
		loc := loc.MustParseLocator(pkg)
		contents := bytes.NewBuffer([]byte(fmt.Sprintf("data%v", i)))
		_, err := s.pack.CreatePackage(loc, contents, pack.WithLabels(pack.TeleportMasterConfigPackageLabels))
		c.Assert(err, check.IsNil)
	}
}

var _ = check.Suite(&importerSuite{})

type importerSuite struct {
	dir     string
	backend storage.Backend
	pack    pack.PackageService
}

func TestReconcilesLabels(t *testing.T) {
	tests := []struct {
		name           string
		node           v1.Node
		requiredLabels map[string]string
		expectedLabels map[string]string
	}{
		{
			name: "reconciliation mode = disabled. Different labels",
			node: nodeWithLabels(map[string]string{
				defaults.KubernetesReconcileLabel: defaults.ReconcileModeDisabled,
			}),
			requiredLabels: map[string]string{
				"label1": "value1",
				"label2": "value2",
			},
			expectedLabels: map[string]string{
				defaults.KubernetesReconcileLabel: defaults.ReconcileModeDisabled,
			},
		},
		{
			name: "reconciliation mode = enabled. Different labels",
			node: nodeWithLabels(map[string]string{
				defaults.KubernetesReconcileLabel: defaults.ReconcileModeEnabled,
				"label1":                          "1",
				"label2":                          "1",
			}),
			requiredLabels: map[string]string{
				"label1": "value1",
				"label2": "value2",
			},
			expectedLabels: map[string]string{
				defaults.KubernetesReconcileLabel: defaults.ReconcileModeEnabled,
				"label1":                          "value1",
				"label2":                          "value2",
			},
		},
		{
			name: "reconciliation mode = enabled. Same labels",
			node: nodeWithLabels(map[string]string{
				defaults.KubernetesReconcileLabel: defaults.ReconcileModeEnabled,
				"label1":                          "value1",
				"label2":                          "value2",
			}),
			requiredLabels: map[string]string{
				"label1": "value1",
				"label2": "value2",
			},
			expectedLabels: map[string]string{
				defaults.KubernetesReconcileLabel: defaults.ReconcileModeEnabled,
				"label1":                          "value1",
				"label2":                          "value2",
			},
		},
		{
			name: "reconciliation mode = EnsureExists. Different labels",
			node: nodeWithLabels(map[string]string{
				defaults.KubernetesReconcileLabel: defaults.ReconcileModeEnsureExists,
			}),
			requiredLabels: map[string]string{
				"label1": "value1",
				"label2": "value2",
			},
			expectedLabels: map[string]string{
				defaults.KubernetesReconcileLabel: defaults.ReconcileModeEnsureExists,
				"label1":                          "value1",
				"label2":                          "value2",
			},
		},
		{
			name: "reconciliation mode = EnsureExists. Different value of labels and no change is expected.",
			node: nodeWithLabels(map[string]string{
				defaults.KubernetesReconcileLabel: defaults.ReconcileModeEnsureExists,
				"label1":                          "1",
				"label2":                          "2",
			}),
			requiredLabels: map[string]string{
				"label1": "value1",
				"label2": "value2",
			},
			expectedLabels: map[string]string{
				defaults.KubernetesReconcileLabel: defaults.ReconcileModeEnsureExists,
				"label1":                          "1",
				"label2":                          "2",
			},
		},
		{
			name: "reconciliation mode is empty. Same labels",
			node: nodeWithLabels(map[string]string{
				"label1": "value1",
				"label2": "value2",
			}),
			requiredLabels: map[string]string{
				"label1": "value1",
				"label2": "value2",
			},
			expectedLabels: map[string]string{
				defaults.KubernetesReconcileLabel: defaults.ReconcileModeEnsureExists,
				"label1":                          "value1",
				"label2":                          "value2",
			},
		},
		{
			name: "reconciliation mode is incorrect. Different value of labels and no change is expected",
			node: nodeWithLabels(map[string]string{
				defaults.KubernetesReconcileLabel: "Incorrect",
				"label1":                          "1",
				"label2":                          "2",
			}),
			requiredLabels: map[string]string{
				"label1": "value1",
				"label2": "value2",
			},
			expectedLabels: map[string]string{
				defaults.KubernetesReconcileLabel: defaults.ReconcileModeEnsureExists,
				"label1":                          "1",
				"label2":                          "2",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logrus.WithField("test", tt.name)
			patcher := &testPatcher{labels: tt.node.Labels}
			err := reconcileNode(context.Background(), tt.requiredLabels, tt.node, patcher, logger)
			if err != nil {
				t.Errorf("failed to reconcile node: %v", err)
			}
			if !reflect.DeepEqual(patcher.labels, tt.expectedLabels) {
				t.Errorf("reconcileLabels() labels = %v, want %v", patcher.labels, tt.expectedLabels)
			}
		})
	}
}

func (r *testPatcher) patch(ctx context.Context, node v1.Node) error {
	r.labels = node.ObjectMeta.Labels
	return nil
}

type testPatcher struct {
	labels map[string]string
}

func nodeWithLabels(labels map[string]string) v1.Node {
	return v1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Node",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node",
			Labels: labels,
		},
	}
}
