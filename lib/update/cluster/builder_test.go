/*
Copyright 2019 Gravitational, Inc.

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

package cluster

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/gravitational/gravity/lib/app"
	appservice "github.com/gravitational/gravity/lib/app/service"
	apptest "github.com/gravitational/gravity/lib/app/service/test"
	"github.com/gravitational/gravity/lib/archive"
	"github.com/gravitational/gravity/lib/constants"
	"github.com/gravitational/gravity/lib/defaults"
	"github.com/gravitational/gravity/lib/fsm"
	"github.com/gravitational/gravity/lib/loc"
	"github.com/gravitational/gravity/lib/ops"
	"github.com/gravitational/gravity/lib/pack"
	"github.com/gravitational/gravity/lib/schema"
	"github.com/gravitational/gravity/lib/storage"
	"github.com/gravitational/gravity/lib/update/cluster/versions"

	"github.com/coreos/go-semver/semver"
	"github.com/google/go-cmp/cmp"
	"gopkg.in/check.v1"
)

type PlanSuite struct {
	clusterAppLoc             loc.Locator
	runtimeAppLoc             loc.Locator
	updateClusterAppLoc       loc.Locator
	updateRuntimeAppLoc       loc.Locator
	intermediateRuntimeAppLoc loc.Locator
	runtimeLoc                loc.Locator
	updateRuntimeLoc          loc.Locator
	intermediateRuntimeLoc    loc.Locator

	operation              storage.SiteOperation
	links                  []storage.OpsCenterLink
	clusterApp             *app.Application
	updateClusterApp       *app.Application
	intermediateRuntimeApp *app.Application
	services               appservice.TestServices
}

func (s *PlanSuite) SetUpSuite(c *check.C) {
	s.clusterAppLoc = newLoc("app:1.0.0")
	s.updateClusterAppLoc = newLoc("app:3.0.0")
	s.runtimeAppLoc = newLoc("kubernetes:1.0.0")
	s.updateRuntimeAppLoc = loc.MustParseLocator("gravitational.io/runtime:3.0.0")
	s.intermediateRuntimeAppLoc = loc.MustParseLocator("gravitational.io/runtime:2.0.0")
	s.runtimeLoc = loc.MustParseLocator("gravitational.io/planet:1.0.0")
	s.updateRuntimeLoc = loc.MustParseLocator("gravitational.io/planet:3.0.0")
	s.intermediateRuntimeLoc = loc.MustParseLocator("gravitational.io/planet:2.0.0")
	s.links = []storage.OpsCenterLink{
		{
			Hostname:   "ops.example.com",
			Type:       storage.OpsCenterRemoteAccessLink,
			RemoteAddr: "ops.example.com:3024",
			APIURL:     "https://ops.example.com:32009",
			Enabled:    true,
		},
	}
}

func (s *PlanSuite) SetUpTest(c *check.C) {
	s.services = appservice.NewTestServices(c.MkDir(), c)
}

func (s *PlanSuite) TearDownTest(*check.C) {
	s.services.Close()
}

func (s *PlanSuite) setupWithRuntimeUpdate(servers []storage.Server, c *check.C) params {
	gravityPackage := newPackage("gravity:1.0.0")
	teleportPackage := newPackage("teleport:1.0.0")
	updateGravityPackage := newPackage("gravity:2.0.0")
	depAppLoc1 := newLoc("dep-app-1:1.0.0")
	depAppLoc2 := newLoc("dep-app-2:1.0.0")
	depApp1 := apptest.SystemApplication(depAppLoc1).Build()
	depApp2 := apptest.SystemApplication(depAppLoc2).Build()
	rbacApp := apptest.SystemApplication(newLoc("rbac-app:1.0.0")).Build()
	runtimeAppLoc := newLoc("kubernetes:1.0.0")
	updateRuntimeAppLoc := newLoc("kubernetes:2.0.0")
	runtimePackageLoc := newLoc("planet:1.0.0")
	updateRuntimePackageLoc := newLoc("planet:2.0.0")
	runtimeAppDep1 := apptest.SystemApplication(newLoc("runtime-app-1:1.0.0")).Build()
	runtimeAppDep2 := apptest.SystemApplication(newLoc("runtime-app-2:1.0.0")).Build()
	runtimeApp := apptest.RuntimeApplication(runtimeAppLoc, runtimePackageLoc).
		WithPackageDependencies(
			newRuntimePackageWithEtcd(runtimePackageLoc, "3.3.2"),
			gravityPackage, teleportPackage,
		).
		WithAppDependencies(rbacApp, runtimeAppDep1, runtimeAppDep2).
		Build()
	clusterApp := apptest.ClusterApplication(s.clusterAppLoc, runtimeApp).
		WithAppDependencies(depApp1, depApp2).
		Build()
	updateDepAppLoc2 := newLoc("dep-app-2:3.0.0")
	updateDepApp2 := apptest.SystemApplication(updateDepAppLoc2).Build()
	updateRbacApp := apptest.SystemApplication(newLoc("rbac-app:2.0.0")).Build()
	updateRuntimeAppDep2 := apptest.SystemApplication(newLoc("runtime-app-2:2.0.0")).Build()
	updateRuntimeApp := apptest.RuntimeApplication(updateRuntimeAppLoc, updateRuntimePackageLoc).
		WithPackageDependencies(
			newRuntimePackageWithEtcd(updateRuntimePackageLoc, "3.3.3"),
			updateGravityPackage, teleportPackage,
		).
		WithAppDependencies(updateRbacApp, runtimeAppDep1, updateRuntimeAppDep2).
		Build()
	updateClusterApp := apptest.ClusterApplication(s.updateClusterAppLoc, updateRuntimeApp).
		WithAppDependencies(depApp1, updateDepApp2).
		Build()

	s.operation = storage.SiteOperation{
		AccountID:  "account-id",
		SiteDomain: "test",
		ID:         "id",
		Type:       ops.OperationUpdate,
		Update: &storage.UpdateOperationState{
			UpdatePackage: s.updateClusterAppLoc.String(),
		},
	}

	var clusterRuntimeApp *app.Application
	s.clusterApp, clusterRuntimeApp = apptest.CreateApplication(apptest.AppRequest{
		App:      clusterApp,
		Apps:     s.services.Apps,
		Packages: s.services.Packages,
	}, c)
	var updateClusterRuntimeApp *app.Application
	s.updateClusterApp, updateClusterRuntimeApp = apptest.CreateApplication(apptest.AppRequest{
		App:      updateClusterApp,
		Apps:     s.services.Apps,
		Packages: s.services.Packages,
	}, c)
	return params{
		planConfig: planConfig{
			servers:   servers,
			apps:      s.services.Apps,
			packages:  s.services.Packages,
			operator:  testOperator,
			operation: &s.operation,
			dnsConfig: storage.DefaultDNSConfig,
			// Use an alternative (other than first) master node as leader
			leadMaster:         &servers[1],
			serviceUser:        &serviceUser,
			userConfig:         userConfig,
			currentEtcdVersion: newVer("3.3.2"),
			links:              s.links,
			installedApp:       clusterApp.Locator(),
			directUpgradeVersions: versions.Versions{
				newVer("1.0.0"),
			},
			// FIXME(dima): this should not be used
			upgradeViaVersions: map[semver.Version]versions.Versions{
				newVer("1.0.0"): {newVer("2.0.0")},
			},
			numParallel: numParallelPhases,
			newID:       testIDs(1),
		},
		gravityPackage: updateGravityPackage.Loc,
		etcdVersion: etcdVersion{
			installed: "3.3.2",
			update:    "3.3.3",
		},
		runtimeUpdates: []loc.Locator{
			updateRbacApp.Locator(),
			updateRuntimeAppDep2.Locator(),
			updateRuntimeApp.Locator(),
		},
		appUpdates: []loc.Locator{
			updateDepApp2.Locator(),
			updateClusterApp.Locator(),
		},
		installedApp:        *s.clusterApp,
		installedRuntimeApp: *clusterRuntimeApp,
		updateApp:           *s.updateClusterApp,
		updateRuntimeApp:    *updateClusterRuntimeApp,
		teleportLoc:         teleportPackage.Loc,
	}
}

func (s *PlanSuite) setupWithIntermediateRuntimeUpdate(servers []storage.Server, c *check.C) params {
	/*
			# installed runtime
			packages:
			    - gravitational.io/gravity:1.0.0
			  apps:
			    - gravitational.io/runtime-dep-1:1.0.0
			    - gravitational.io/runtime-dep-2:1.0.0
			    - gravitational.io/rbac-app:1.0.0

			# intermediate runtime
			packages:
			    - gravitational.io/gravity:2.0.0
			  apps:
			    - gravitational.io/runtime-dep-1:1.0.0
			    - gravitational.io/runtime-dep-2:2.0.0
			    - gravitational.io/rbac-app:2.0.0

			# update runtime
			packages:
			    - gravitational.io/gravity:3.0.0
			  apps:
			    - gravitational.io/runtime-dep-1:1.0.0
			    - gravitational.io/runtime-dep-2:3.0.0
			    - gravitational.io/rbac-app:3.0.0

		    # installed app
		    - gravitational.io/app-dep-1:1.0.0
		    - gravitational.io/app-dep-2:1.0.0

		    # update app
		    - gravitational.io/app-dep-1:1.0.0
		    - gravitational.io/app-dep-2:3.0.0
	*/

	gravityLoc := newLoc("gravity:1.0.0")
	intermediateGravityLoc := newLoc("gravity:2.0.0")
	updateGravityLoc := newLoc("gravity:3.0.0")
	depAppLoc1 := newLoc("dep-app-1:1.0.0")
	depAppLoc2 := newLoc("dep-app-2:1.0.0")
	depApp1 := apptest.SystemApplication(depAppLoc1).Build()
	depApp2 := apptest.SystemApplication(depAppLoc2).Build()
	runtimeApp := apptest.RuntimeApplication(apptest.RuntimeApplicationLoc, apptest.RuntimePackageLoc).
		WithSchemaPackageDependencies(gravityLoc).
		Build()
	clusterApp := apptest.ClusterApplication(s.clusterAppLoc, runtimeApp).
		WithAppDependencies(depApp1, depApp2).
		Build()
	intermediateRuntimeApp := apptest.RuntimeApplication(
		s.intermediateRuntimeAppLoc, s.intermediateRuntimeLoc).
		WithSchemaPackageDependencies(intermediateGravityLoc).
		Build()
	updateDepAppLoc2 := newLoc("dep-app-2:3.0.0")
	updateDepApp2 := apptest.SystemApplication(updateDepAppLoc2).Build()
	updateRuntimeApp := apptest.RuntimeApplication(apptest.RuntimeApplicationLoc, apptest.RuntimePackageLoc).
		WithSchemaPackageDependencies(updateGravityLoc).
		Build()
	updateClusterApp := apptest.ClusterApplication(s.updateClusterAppLoc, updateRuntimeApp).
		WithAppDependencies(depApp1, updateDepApp2).
		Build()
	s.clusterApp, _ = apptest.CreateApplication(apptest.AppRequest{
		App:      clusterApp,
		Apps:     s.services.Apps,
		Packages: s.services.Packages,
	}, c)
	s.intermediateRuntimeApp, _ = apptest.CreateApplication(apptest.AppRequest{
		App:      intermediateRuntimeApp,
		Apps:     s.services.Apps,
		Packages: s.services.Packages,
	}, c)
	s.updateClusterApp, _ = apptest.CreateApplication(apptest.AppRequest{
		App:      updateClusterApp,
		Apps:     s.services.Apps,
		Packages: s.services.Packages,
	}, c)
	// TODO(dima)
	return params{
		gravityPackage: updateGravityLoc,
		//runtimeUpdates: []loc.Locator{},
	}
}

var _ = check.Suite(&PlanSuite{})

func (s *PlanSuite) TestPlanWithRuntimeAppsUpdate(c *check.C) {
	servers := []storage.Server{
		newMaster("node-1"),
		newMaster("node-2"),
		newWorker("node-3"),
	}
	// TODO(dima): init idGen inside the test
	params := s.setupWithRuntimeUpdate(servers, c)

	// exercise
	obtainedPlan, err := newOperationPlan(context.Background(), params.planConfig)
	c.Assert(err, check.IsNil)

	// verify
	updates := params.updates()
	leadMaster := updates[1]
	rearrangedServers := []storage.UpdateServer{updates[1], updates[0], updates[2]}
	expectedPlan := storage.OperationPlan{
		OperationID:        s.operation.ID,
		OperationType:      s.operation.Type,
		AccountID:          s.operation.AccountID,
		ClusterName:        s.operation.SiteDomain,
		Servers:            servers,
		DNSConfig:          storage.DefaultDNSConfig,
		GravityPackage:     params.gravityPackage,
		OfflineCoordinator: params.planConfig.leadMaster,
		Phases: []storage.OperationPhase{
			params.init(rearrangedServers),
			params.checks("/init"),
			params.preUpdate("/init", "/checks"),
			params.bootstrap(rearrangedServers, params.gravityPackage, "/checks", "/pre-update"),
			params.coreDNS("/bootstrap"),
			params.masters(leadMaster, updates[0:1], params.gravityPackage, "1", "/coredns"),
			params.nodes(updates[2:], leadMaster.Server, params.gravityPackage, "1", "/masters"),
			params.etcd(leadMaster.Server, updates[0:1], params.etcdVersion),
			params.config("/etcd"),
			params.runtime(params.runtimeUpdates, "/config"),
			params.migration("/runtime"),
			params.app("/migration"),
			params.cleanup(),
		},
	}
	if !cmp.Equal(*obtainedPlan, expectedPlan) {
		c.Error("Plans differ:", cmp.Diff(*obtainedPlan, expectedPlan))
	}
}

/*
func (s *PlanSuite) TestPlanWithRuntimeAppsUpdate0(c *check.C) {
	// setup
	installedRuntimeApp := newApp("gravitational.io/runtime:1.0.0", installedRuntimeAppManifest)
	installedApp := newApp("gravitational.io/app:1.0.0", installedAppManifest)
	updateRuntimeApp := newApp("gravitational.io/runtime:2.0.0", updateRuntimeAppManifest)
	updateApp := newApp("gravitational.io/app:2.0.0", updateAppManifest)
	gravityPackage := mustLocator(updateRuntimeApp.Manifest.Dependencies.ByName(
		constants.GravityPackage))
	servers := []storage.Server{
		{
			AdvertiseIP: "192.168.0.1",
			Hostname:    "node-1",
			Role:        "node",
			ClusterRole: string(schema.ServiceRoleMaster),
		},
		{
			AdvertiseIP: "192.168.0.2",
			Hostname:    "node-2",
			Role:        "node",
			ClusterRole: string(schema.ServiceRoleMaster),
		},
		{
			AdvertiseIP: "192.168.0.3",
			Hostname:    "node-3",
			Role:        "node",
			ClusterRole: string(schema.ServiceRoleNode),
		},
	}
	updates := []storage.UpdateServer{
		{
			Server:  servers[0],
			Runtime: runtimePackage,
		},
		{
			Server:  servers[1],
			Runtime: runtimePackage,
		},
		{
			Server:  servers[2],
			Runtime: runtimePackage,
		},
	}
	runtimeUpdates := []loc.Locator{
		loc.MustParseLocator("gravitational.io/rbac-app:2.0.0"),
		loc.MustParseLocator("gravitational.io/runtime-dep-2:2.0.0"),
		updateRuntimeApp.Package,
	}
	params := params{
		servers:             servers,
		installedRuntimeApp: installedRuntimeApp,
		installedApp:        installedApp,
		updateRuntimeApp:    updateRuntimeApp,
		updateApp:           updateApp,
		links: []storage.OpsCenterLink{
			{
				Hostname:   "ops.example.com",
				Type:       storage.OpsCenterRemoteAccessLink,
				RemoteAddr: "ops.example.com:3024",
				APIURL:     "https://ops.example.com:32009",
				Enabled:    true,
			},
		},
		dnsConfig: storage.DefaultDNSConfig,
		// Use an alternative (other than first) master node as leader
		leadMaster: servers[1],
		appUpdates: []loc.Locator{
			loc.MustParseLocator("gravitational.io/app-dep-2:2.0.0"),
			updateApp.Package,
		},
		targetStep: newTargetUpdateStep(updateStep{
			runtimeUpdates: runtimeUpdates,
			etcd: &etcdVersion{
				installed: "1.0.0",
				update:    "2.0.0",
			},
			servers:     updates,
			gravity:     gravityPackage,
			changesetID: "id",
			userConfig:  UserConfig{ParallelWorkers: numParallelWorkers},
			numParallel: numParallelPhases,
		}),
	}
	builder := newBuilder(c, params)

	// exercise
	obtainedPlan := builder.newPlan()

	// verify
	leadMaster := updates[1]
	rearrangedServers := []storage.UpdateServer{updates[1], updates[0], updates[2]}
	expectedPlan := storage.OperationPlan{
		OperationID:        builder.operation.ID,
		OperationType:      builder.operation.Type,
		AccountID:          builder.operation.AccountID,
		ClusterName:        builder.operation.SiteDomain,
		Servers:            servers,
		DNSConfig:          storage.DefaultDNSConfig,
		GravityPackage:     loc.MustParseLocator("gravitational.io/gravity:3.0.0"),
		OfflineCoordinator: &params.leadMaster,
		Phases: []storage.OperationPhase{
			params.init(rearrangedServers),
			params.checks("/init"),
			params.preUpdate("/init", "/checks"),
			params.bootstrap(rearrangedServers, gravityPackage, "/checks", "/pre-update"),
			params.coreDNS("/bootstrap"),
			params.masters(leadMaster, updates[0:1], gravityPackage, "id", "/coredns"),
			params.nodes(updates[2:], leadMaster.Server, gravityPackage, "id", "/masters"),
			params.etcd(leadMaster.Server, updates[0:1], *params.targetStep.etcd),
			params.config("/etcd"),
			params.runtime(runtimeUpdates, "/config"),
			params.migration("/runtime"),
			params.app("/migration"),
			params.cleanup(),
		},
	}
	if !cmp.Equal(*obtainedPlan, expectedPlan) {
		c.Error("Plans differ:", cmp.Diff(*obtainedPlan, expectedPlan))
	}
}

func (s *PlanSuite) TestPlanWithoutRuntimeAppUpdate(c *check.C) {
	// setup
	installedRuntimeApp := newApp("gravitational.io/runtime:1.0.0", installedRuntimeAppManifest)
	installedApp := newApp("gravitational.io/app:1.0.0", installedAppManifest)
	updateApp := newApp("gravitational.io/app:2.0.0", updateAppManifest)
	servers := []storage.Server{
		{
			AdvertiseIP: "192.168.0.1",
			Hostname:    "node-1",
			Role:        "node",
			ClusterRole: string(schema.ServiceRoleMaster),
		},
		{
			AdvertiseIP: "192.168.0.2",
			Hostname:    "node-2",
			Role:        "node",
			ClusterRole: string(schema.ServiceRoleMaster),
		},
		{
			AdvertiseIP: "192.168.0.3",
			Hostname:    "node-3",
			Role:        "node",
			ClusterRole: string(schema.ServiceRoleNode),
		},
	}
	params := params{
		servers:             servers,
		installedRuntimeApp: installedRuntimeApp,
		installedApp:        installedApp,
		updateRuntimeApp:    installedRuntimeApp, // same runtime on purpose
		updateApp:           updateApp,
		dnsConfig:           storage.DefaultDNSConfig,
		leadMaster:          servers[0],
		appUpdates: []loc.Locator{
			loc.MustParseLocator("gravitational.io/app-dep-2:2.0.0"),
			updateApp.Package,
		},
	}
	builder := newBuilder(c, params)

	// exercise
	obtainedPlan := builder.newPlan()

	// verify
	c.Assert(*obtainedPlan, check.DeepEquals, storage.OperationPlan{
		OperationID:        builder.operation.ID,
		OperationType:      builder.operation.Type,
		AccountID:          builder.operation.AccountID,
		ClusterName:        builder.operation.SiteDomain,
		Servers:            servers,
		DNSConfig:          storage.DefaultDNSConfig,
		GravityPackage:     gravityInstalledLoc,
		OfflineCoordinator: &params.leadMaster,
		Phases: []storage.OperationPhase{
			params.checks(),
			params.preUpdate("/checks"),
			params.app("/pre-update"),
			params.cleanup(),
		},
	})
}

func (s *PlanSuite) TestPlanWithIntermediateRuntimeUpdate(c *check.C) {
	// setup
	installedRuntimeApp := newApp("gravitational.io/runtime:1.0.0", installedRuntimeAppManifest)
	installedApp := newApp("gravitational.io/app:1.0.0", installedAppManifest)
	intermediateRuntimeApp := newApp("gravitational.io/runtime:2.0.0", intermediateRuntimeAppManifest)
	updateRuntimeApp := newApp("gravitational.io/runtime:3.0.0", updateRuntimeAppManifest)
	updateApp := newApp("gravitational.io/app:3.0.0", updateAppManifest)
	intermediateGravityPackage := mustLocator(intermediateRuntimeApp.Manifest.Dependencies.ByName(
		constants.GravityPackage))
	gravityPackage := mustLocator(updateRuntimeApp.Manifest.Dependencies.ByName(
		constants.GravityPackage))
	servers := []storage.Server{
		{
			AdvertiseIP: "192.168.0.1",
			Hostname:    "node-1",
			Role:        "node",
			ClusterRole: string(schema.ServiceRoleMaster),
		},
		{
			AdvertiseIP: "192.168.0.2",
			Hostname:    "node-2",
			Role:        "node",
			ClusterRole: string(schema.ServiceRoleMaster),
		},
		{
			AdvertiseIP: "192.168.0.3",
			Hostname:    "node-3",
			Role:        "node",
			ClusterRole: string(schema.ServiceRoleNode),
		},
	}
	intermediateUpdates := []storage.UpdateServer{
		{
			Server:  servers[0],
			Runtime: intermediateRuntimePackage,
		},
		{
			Server:  servers[1],
			Runtime: intermediateRuntimePackage,
		},
		{
			Server:  servers[2],
			Runtime: intermediateRuntimePackage,
		},
	}
	updates := []storage.UpdateServer{
		{
			Server:  servers[0],
			Runtime: runtimePackage,
		},
		{
			Server:  servers[1],
			Runtime: runtimePackage,
		},
		{
			Server:  servers[2],
			Runtime: runtimePackage,
		},
	}
	intermediateRuntimeUpdates := []loc.Locator{intermediateRuntimeApp.Package}
	runtimeUpdates := []loc.Locator{
		loc.MustParseLocator("gravitational.io/rbac-app:2.0.0"),
		loc.MustParseLocator("gravitational.io/runtime-dep-2:2.0.0"),
		updateRuntimeApp.Package,
	}
	params := params{
		servers:             servers,
		installedRuntimeApp: installedRuntimeApp,
		installedApp:        installedApp,
		updateRuntimeApp:    updateRuntimeApp,
		updateApp:           updateApp,
		links: []storage.OpsCenterLink{
			{
				Hostname:   "ops.example.com",
				Type:       storage.OpsCenterRemoteAccessLink,
				RemoteAddr: "ops.example.com:3024",
				APIURL:     "https://ops.example.com:32009",
				Enabled:    true,
			},
		},
		dnsConfig: storage.DefaultDNSConfig,
		// Use an alternative (other than first) master node as leader
		leadMaster: servers[1],
		appUpdates: []loc.Locator{
			loc.MustParseLocator("gravitational.io/app-dep-2:2.0.0"),
			updateApp.Package,
		},
		steps: []intermediateUpdateStep{
			{
				updateStep: updateStep{
					changesetID: "id2",
					servers:     intermediateUpdates,
					etcd: &etcdVersion{
						installed: "1.0.0",
						update:    "2.0.0",
					},
					runtimeUpdates: intermediateRuntimeUpdates,
					gravity:        intermediateGravityPackage,
					userConfig:     UserConfig{ParallelWorkers: numParallelWorkers},
					numParallel:    numParallelPhases,
				},
				runtimeAppVersion: *semver.New("1.0.0"),
			},
		},
		targetStep: targetUpdateStep{updateStep: updateStep{
			changesetID:    "id",
			runtimeUpdates: runtimeUpdates,
			etcd: &etcdVersion{
				installed: "2.0.0",
				update:    "3.0.0",
			},
			gravity:     gravityPackage,
			servers:     updates,
			userConfig:  UserConfig{ParallelWorkers: numParallelWorkers},
			numParallel: numParallelPhases,
		}},
	}
	builder := newBuilder(c, params)

	// exercise
	obtainedPlan := builder.newPlan()

	// verify
	intermediateLeadMaster := intermediateUpdates[1]
	rearrangedIntermediateServers := []storage.UpdateServer{intermediateUpdates[1], intermediateUpdates[0], intermediateUpdates[2]}
	intermediateOtherMasters := intermediateUpdates[0:1]
	intermediateNodes := intermediateUpdates[2:]
	leadMaster := updates[1]
	rearrangedServers := []storage.UpdateServer{updates[1], updates[0], updates[2]}
	otherMasters := updates[0:1]
	nodes := updates[2:]

	expectedPlan := storage.OperationPlan{
		OperationID:        builder.operation.ID,
		OperationType:      builder.operation.Type,
		AccountID:          builder.operation.AccountID,
		ClusterName:        builder.operation.SiteDomain,
		Servers:            servers,
		DNSConfig:          storage.DefaultDNSConfig,
		GravityPackage:     loc.MustParseLocator("gravitational.io/gravity:3.0.0"),
		OfflineCoordinator: &params.leadMaster,
		Phases: []storage.OperationPhase{
			params.init(rearrangedIntermediateServers),
			params.checks("/init"),
			params.preUpdate("/init", "/checks"),
			params.sub("/1.0.0", []string{"/checks", "/pre-update"},
				params.bootstrapVersioned(rearrangedIntermediateServers, "1.0.0", intermediateGravityPackage),
				params.masters(intermediateLeadMaster, intermediateOtherMasters, intermediateGravityPackage, "id2", "/bootstrap"),
				params.nodes(intermediateNodes, intermediateLeadMaster.Server, intermediateGravityPackage, "id2", "/masters"),
				params.etcd(intermediateLeadMaster.Server,
					intermediateOtherMasters,
					*params.steps[0].etcd),
				params.config("/etcd"),
				params.runtime(intermediateRuntimeUpdates, "/config"),
			),
			params.sub("/target", []string{"/1.0.0"},
				params.bootstrap(rearrangedServers, gravityPackage),
				params.coreDNS("/bootstrap"),
				params.masters(leadMaster, otherMasters, gravityPackage, "id", "/coredns"),
				params.nodes(nodes, leadMaster.Server, gravityPackage, "id", "/masters"),
				params.etcd(leadMaster.Server, otherMasters, *params.targetStep.etcd),
				params.config("/etcd"),
				params.runtime(runtimeUpdates, "/config"),
			),
			params.migration("/target"),
			params.app("/migration"),
			params.cleanup(),
		},
	}

	if !cmp.Equal(*obtainedPlan, expectedPlan) {
		c.Error("Plans differ:", cmp.Diff(*obtainedPlan, expectedPlan))
	}
}

func (s *PlanSuite) TestDeterminesWhetherToUpdateEtcd(c *check.C) {
	services := opsservice.SetupTestServices(c)
	defer services.Close()
	runtimePackageLoc := loc.MustParseLocator("gravitational.io/runtime:1.0.0")
	runtimeAppLoc := loc.MustParseLocator("gravitational.io/base:1.0.0")
	runtimeApp := apptest.RuntimeApplication(runtimeAppLoc, runtimePackageLoc).
		WithPackageDependencies(apptest.Package{
			Loc: runtimePackageLoc,
			Items: []*archive.Item{
				archive.ItemFromString("orbit.manifest.json", `{
	"version": "0.0.1",
	"labels": [
		{
			"name": "version-etcd",
			"value": "v3.3.2"
		}
	]
}`),
			}}).
		Build()
	clusterAppLoc := loc.MustParseLocator("gravitational.io/app:1.0.0")
	clusterApp := apptest.CreateApplication(apptest.AppRequest{
		App:      apptest.ClusterApplication(clusterAppLoc, runtimeApp).Build(),
		Apps:     services.Apps,
		Packages: services.Packages,
	}, c)
	updateRuntimePackageLoc := loc.MustParseLocator("gravitational.io/runtime:2.0.0")
	updateRuntimeAppLoc := loc.MustParseLocator("gravitational.io/base:2.0.0")
	updateRuntimeApp := apptest.RuntimeApplication(updateRuntimeAppLoc, updateRuntimePackageLoc).
		WithPackageDependencies(apptest.Package{
			Loc: updateRuntimePackageLoc,
			Items: []*archive.Item{
				archive.ItemFromString("orbit.manifest.json", `{
	"version": "0.0.1",
	"labels": [
		{
			"name": "version-etcd",
			"value": "v3.3.3"
		}
	]
}`),
			}}).Build()
	updateClusterAppLoc := loc.MustParseLocator("gravitational.io/app:2.0.0")
	updateClusterApp := apptest.CreateApplication(apptest.AppRequest{
		App:      apptest.ClusterApplication(updateClusterAppLoc, updateRuntimeApp).Build(),
		Apps:     services.Apps,
		Packages: services.Packages,
	}, c)

	b := phaseBuilder{
		operator: testOperator,
		planTemplate: storage.OperationPlan{
			Servers: []storage.Server{{
				AdvertiseIP: "192.168.0.1",
				Hostname:    "node-1",
				Role:        "node",
				ClusterRole: string(schema.ServiceRoleMaster),
			}},
		},
		packages:            services.Packages,
		apps:                services.Apps,
		installedRuntimeApp: runtimeApp.App(),
		installedApp:        *clusterApp,
		updateRuntimeApp:    updateRuntimeApp.App(),
		updateApp:           *updateClusterApp,
		installedTeleport:   loc.MustParseLocator("gravitational.io/teleport:1.0.0"),
		updateTeleport:      loc.MustParseLocator("gravitational.io/teleport:2.0.0"),
	}
	err := b.initSteps(context.Background())
	c.Assert(err, check.IsNil)

	c.Assert(b.targetStep.etcd, check.DeepEquals, &etcdVersion{
		installed: "3.3.2",
		update:    "3.3.3",
	})
}
*/

//func (r *params) newPlan(c *check.C) *storage.OperationPlan {
//	plan, err := newOperationPlan(context.Background(), planConfig{
//		Packages:        r.packages,
//		Apps:            r.apps,
//		Operator:        r.operator,
//		DNSConfig:       r.dnsConfig,
//		Operation:       &r.operation,
//		Leader:          r.leadServer,
//		ServiceUser:     r.serviceUser,
//		UserConfig:      r.userConfig,
//		links:           r.links,
//		roles:           r.roles,
//		trustedClusters: r.trustedClusters,
//		servers:         r.servers,
//		installedApp:    r.installedApp,
//		//CurrentEtcdVersion: r.currentEtcdVersion,
//	})
//	c.Assert(err, check.IsNil)
//	return plan
//}

//func (r *params) newBuilder(c *check.C) phaseBuilder {
//	builder := phaseBuilder{
//		operator:            r.operator,
//		operation:           r.operation,
//		installedRuntimeApp: r.installedRuntimeApp,
//		installedApp:        r.installedApp,
//		updateRuntimeApp:    r.updateRuntimeApp,
//		updateApp:           r.updateApp,
//		links:               r.links,
//		trustedClusters:     r.trustedClusters,
//		leadMaster:          r.leadMaster,
//		//steps:               params.steps,
//		//targetStep:          params.targetStep,
//		numParallel: numParallelPhases,
//	}
//	gravityPackage, err := builder.updateRuntimeApp.Manifest.Dependencies.ByName(
//		constants.GravityPackage)
//	c.Assert(err, check.IsNil)
//	builder.planTemplate = storage.OperationPlan{
//		OperationID:        builder.operation.ID,
//		OperationType:      builder.operation.Type,
//		AccountID:          builder.operation.AccountID,
//		ClusterName:        builder.operation.SiteDomain,
//		Servers:            params.servers,
//		GravityPackage:     *gravityPackage,
//		DNSConfig:          params.dnsConfig,
//		OfflineCoordinator: &params.leadMaster,
//	}
//	return builder
//}

func (r *params) sub(id string, requires []string, phases ...storage.OperationPhase) storage.OperationPhase {
	parentize(id, phases)
	return storage.OperationPhase{
		ID:       id,
		Phases:   phases,
		Requires: requires,
	}
}

func parentize(parentID string, phases []storage.OperationPhase) {
	for i, phase := range phases {
		phases[i].ID = path.Join(parentID, phase.ID)
		for j, req := range phase.Requires {
			phases[i].Requires[j] = path.Join(parentID, req)
		}
		if len(phase.Phases) != 0 {
			parentize(parentID, phase.Phases)
		}
	}
}

func (r *params) init(servers []storage.UpdateServer) storage.OperationPhase {
	root := storage.OperationPhase{
		ID:          "/init",
		Description: "Initialize update operation",
	}
	leadMaster := servers[0]
	root.Phases = append(root.Phases, storage.OperationPhase{
		ID:          fmt.Sprintf("/init/%v", leadMaster.Hostname),
		Executor:    updateInitLeader,
		Description: fmt.Sprintf("Initialize node %q", leadMaster.Hostname),
		Data: &storage.OperationPhaseData{
			ExecServer:       &leadMaster.Server,
			Package:          &r.updateApp.Package,
			InstalledPackage: &r.installedApp.Package,
			Update: &storage.UpdateOperationData{
				Servers: []storage.UpdateServer{leadMaster},
			},
		},
	})
	for _, server := range servers[1:] {
		root.Phases = append(root.Phases, r.initServer(server))
	}
	return root
}

func (r *params) initServer(server storage.UpdateServer) storage.OperationPhase {
	return storage.OperationPhase{
		ID:          fmt.Sprintf("/init/%v", server.Hostname),
		Executor:    updateInit,
		Description: fmt.Sprintf("Initialize node %q", server.Hostname),
		Data: &storage.OperationPhaseData{
			ExecServer: &server.Server,
			Update: &storage.UpdateOperationData{
				Servers: []storage.UpdateServer{server},
			},
		},
	}
}

func (r *params) checks(requires ...string) storage.OperationPhase {
	return storage.OperationPhase{
		ID:          "/checks",
		Executor:    updateChecks,
		Description: "Run preflight checks",
		Requires:    requires,
		Data: &storage.OperationPhaseData{
			Package:          &r.updateApp.Package,
			InstalledPackage: &r.installedApp.Package,
		},
	}
}

func (r *params) preUpdate(requires ...string) storage.OperationPhase {
	return storage.OperationPhase{
		ID:          "/pre-update",
		Executor:    preUpdate,
		Description: "Run pre-update application hook",
		Requires:    requires,
		Data: &storage.OperationPhaseData{
			Package: &r.updateApp.Package,
		},
	}
}

func (r *params) coreDNS(requires ...string) storage.OperationPhase {
	return storage.OperationPhase{
		ID:          "/coredns",
		Description: "Provision CoreDNS resources",
		Executor:    coredns,
		Requires:    requires,
		Data: &storage.OperationPhaseData{
			Server: r.planConfig.leadMaster,
		},
	}
}

func (r *params) masters(leadMaster storage.UpdateServer, otherMasters []storage.UpdateServer, gravityPackage loc.Locator, changesetID string, requires ...string) storage.OperationPhase {
	return storage.OperationPhase{
		ID:          "/masters",
		Description: "Update master nodes",
		Requires:    requires,
		Phases: []storage.OperationPhase{
			r.leaderMasterPhase("/masters", leadMaster, otherMasters, gravityPackage, changesetID),
			r.otherMasterPhase(otherMasters[0], "/masters", leadMaster.Server, gravityPackage, changesetID),
		},
	}
}

func (r *params) leaderMasterPhase(parent string, leadMaster storage.UpdateServer, otherMasters []storage.UpdateServer, gravityPackage loc.Locator, changesetID string) storage.OperationPhase {
	p := func(format string) string {
		return fmt.Sprintf(path.Join(parent, format), leadMaster.Hostname)
	}
	t := func(format string) string {
		return fmt.Sprintf(format, leadMaster.Hostname)
	}
	result := storage.OperationPhase{
		ID:          p("%v"),
		Description: t("Update system software on master node %q"),
		Phases: []storage.OperationPhase{
			{
				ID:          p("%v/kubelet-permissions"),
				Description: t("Add permissions to kubelet on %q"),
				Executor:    kubeletPermissions,
				Data: &storage.OperationPhaseData{
					Server: &leadMaster.Server,
				},
			},
			{
				ID:          p("%v/stepdown"),
				Executor:    electionStatus,
				Description: t("Step down %q as Kubernetes leader"),
				Data: &storage.OperationPhaseData{
					Server: &leadMaster.Server,
					ElectionChange: &storage.ElectionChange{
						DisableServers: []storage.Server{leadMaster.Server},
					},
				},
				Requires: []string{p("%v/kubelet-permissions")},
			},
			{
				ID:          p("%v/drain"),
				Executor:    drainNode,
				Description: t("Drain node %q"),
				Data: &storage.OperationPhaseData{
					Server:     &leadMaster.Server,
					ExecServer: &leadMaster.Server,
				},
				Requires: []string{p("%v/stepdown")},
			},
			{
				ID:          p("%v/system-upgrade"),
				Executor:    updateSystem,
				Description: t("Update system software on node %q"),
				Data: &storage.OperationPhaseData{
					ExecServer: &leadMaster.Server,
					Update: &storage.UpdateOperationData{
						Servers:        []storage.UpdateServer{leadMaster},
						GravityPackage: &gravityPackage,
						ChangesetID:    changesetID,
					},
				},
				Requires: []string{p("%v/drain")},
			},
			{
				ID:          p("%v/elect"),
				Executor:    electionStatus,
				Description: t("Make node %q Kubernetes leader"),
				Data: &storage.OperationPhaseData{
					Server: &leadMaster.Server,
					ElectionChange: &storage.ElectionChange{
						EnableServers:  []storage.Server{leadMaster.Server},
						DisableServers: serversToStorage(otherMasters...),
					},
				},
				Requires: []string{p("%v/system-upgrade")},
			},
			{
				ID:          p("%v/health"),
				Executor:    nodeHealth,
				Description: t("Health check node %q"),
				Data: &storage.OperationPhaseData{
					Server: &leadMaster.Server,
				},
				Requires: []string{p("%v/elect")},
			},
			{
				ID:          p("%v/taint"),
				Executor:    taintNode,
				Description: t("Taint node %q"),
				Data: &storage.OperationPhaseData{
					Server:     &leadMaster.Server,
					ExecServer: &leadMaster.Server,
				},
				Requires: []string{p("%v/health")},
			},
			{
				ID:          p("%v/uncordon"),
				Executor:    uncordonNode,
				Description: t("Uncordon node %q"),
				Data: &storage.OperationPhaseData{
					Server:     &leadMaster.Server,
					ExecServer: &leadMaster.Server,
				},
				Requires: []string{p("%v/taint")},
			},
			{
				ID:          p("%v/untaint"),
				Executor:    untaintNode,
				Description: t("Remove taint from node %q"),
				Data: &storage.OperationPhaseData{
					Server:     &leadMaster.Server,
					ExecServer: &leadMaster.Server,
				},
				Requires: []string{p("%v/uncordon")},
			},
		},
	}
	return result
}

func (r *params) otherMasterPhase(server storage.UpdateServer, parent string, leadMaster storage.Server, gravityPackage loc.Locator, changesetID string) storage.OperationPhase {
	p := func(format string) string {
		return fmt.Sprintf(path.Join(parent, format), server.Hostname)
	}
	t := func(format string) string {
		return fmt.Sprintf(format, server.Hostname)
	}
	return storage.OperationPhase{
		ID:          p("%v"),
		Description: t("Update system software on master node %q"),
		Requires:    []string{fmt.Sprintf("%v/%v", parent, leadMaster.Hostname)},
		Phases: []storage.OperationPhase{
			{
				ID:          p("%v/drain"),
				Executor:    drainNode,
				Description: t("Drain node %q"),
				Data: &storage.OperationPhaseData{
					Server:     &server.Server,
					ExecServer: &leadMaster,
				},
			},
			{
				ID:          p("%v/system-upgrade"),
				Executor:    updateSystem,
				Description: t("Update system software on node %q"),
				Data: &storage.OperationPhaseData{
					ExecServer: &server.Server,
					Update: &storage.UpdateOperationData{
						Servers:        []storage.UpdateServer{server},
						GravityPackage: &gravityPackage,
						ChangesetID:    changesetID,
					},
				},
				Requires: []string{p("%v/drain")},
			},
			{
				ID:          p("%v/elect"),
				Executor:    electionStatus,
				Description: t("Enable leader election on node %q"),
				Data: &storage.OperationPhaseData{
					Server: &server.Server,
					ElectionChange: &storage.ElectionChange{
						EnableServers: []storage.Server{server.Server},
					},
				},
				Requires: []string{p("%v/system-upgrade")},
			},
			{
				ID:          p("%v/health"),
				Executor:    nodeHealth,
				Description: t("Health check node %q"),
				Data: &storage.OperationPhaseData{
					Server: &server.Server,
				},
				Requires: []string{p("%v/elect")},
			},
			{
				ID:          p("%v/taint"),
				Executor:    taintNode,
				Description: t("Taint node %q"),
				Data: &storage.OperationPhaseData{
					Server:     &server.Server,
					ExecServer: &leadMaster,
				},
				Requires: []string{p("%v/health")},
			},
			{
				ID:          p("%v/uncordon"),
				Executor:    uncordonNode,
				Description: t("Uncordon node %q"),
				Data: &storage.OperationPhaseData{
					Server:     &server.Server,
					ExecServer: &leadMaster,
				},
				Requires: []string{p("%v/taint")},
			},
			{
				ID:          p("%v/endpoints"),
				Executor:    endpoints,
				Description: t("Wait for DNS/cluster endpoints on %q"),
				Data: &storage.OperationPhaseData{
					Server:     &server.Server,
					ExecServer: &leadMaster,
				},
				Requires: []string{p("%v/uncordon")},
			},
			{
				ID:          p("%v/untaint"),
				Executor:    untaintNode,
				Description: t("Remove taint from node %q"),
				Data: &storage.OperationPhaseData{
					Server:     &server.Server,
					ExecServer: &leadMaster,
				},
				Requires: []string{p("%v/endpoints")},
			},
		},
	}
}

func (r *params) nodes(updates []storage.UpdateServer, leadMaster storage.Server, gravityPackage loc.Locator, changesetID string, requires ...string) storage.OperationPhase {
	return storage.OperationPhase{
		ID:          "/nodes",
		Description: "Update regular nodes",
		Requires:    requires,
		Phases: []storage.OperationPhase{
			r.nodePhase(updates[0], leadMaster, gravityPackage, "/nodes", changesetID),
		},
		LimitParallel: numParallelWorkers,
	}
}

func (r *params) nodePhase(server storage.UpdateServer, leadMaster storage.Server, gravityPackage loc.Locator, parent, id string) storage.OperationPhase {
	p := func(format string) string {
		return fmt.Sprintf(path.Join(parent, format), server.Hostname)
	}
	t := func(format string) string {
		return fmt.Sprintf(format, server.Hostname)
	}
	return storage.OperationPhase{
		ID:          p("%v"),
		Description: t("Update system software on node %q"),
		Phases: []storage.OperationPhase{
			{
				ID:          p("%v/drain"),
				Executor:    drainNode,
				Description: t("Drain node %q"),
				Data: &storage.OperationPhaseData{
					Server:     &server.Server,
					ExecServer: &leadMaster,
				},
			},
			{
				ID:          p("%v/system-upgrade"),
				Executor:    updateSystem,
				Description: t("Update system software on node %q"),
				Data: &storage.OperationPhaseData{
					ExecServer: &server.Server,
					Update: &storage.UpdateOperationData{
						Servers:        []storage.UpdateServer{server},
						GravityPackage: &gravityPackage,
						ChangesetID:    id,
					},
				},
				Requires: []string{p("%v/drain")},
			},
			{
				ID:          p("%v/health"),
				Executor:    nodeHealth,
				Description: t("Health check node %q"),
				Data: &storage.OperationPhaseData{
					Server: &server.Server,
				},
				Requires: []string{p("%v/system-upgrade")},
			},
			{
				ID:          p("%v/taint"),
				Executor:    taintNode,
				Description: t("Taint node %q"),
				Data: &storage.OperationPhaseData{
					Server:     &server.Server,
					ExecServer: &leadMaster,
				},
				Requires: []string{p("%v/health")},
			},
			{
				ID:          p("%v/uncordon"),
				Executor:    uncordonNode,
				Description: t("Uncordon node %q"),
				Data: &storage.OperationPhaseData{
					Server:     &server.Server,
					ExecServer: &leadMaster,
				},
				Requires: []string{p("%v/taint")},
			},
			{
				ID:          p("%v/endpoints"),
				Executor:    endpoints,
				Description: t("Wait for DNS/cluster endpoints on %q"),
				Data: &storage.OperationPhaseData{
					Server:     &server.Server,
					ExecServer: &leadMaster,
				},
				Requires: []string{p("%v/uncordon")},
			},
			{
				ID:          p("%v/untaint"),
				Executor:    untaintNode,
				Description: t("Remove taint from node %q"),
				Data: &storage.OperationPhaseData{
					Server:     &server.Server,
					ExecServer: &leadMaster,
				},
				Requires: []string{p("%v/endpoints")},
			},
		},
	}
}

func (r *params) bootstrap(servers []storage.UpdateServer, gravityPackage loc.Locator, requires ...string) storage.OperationPhase {
	root := storage.OperationPhase{
		ID:            "/bootstrap",
		Description:   "Bootstrap update operation on nodes",
		Requires:      requires,
		LimitParallel: numParallelPhases,
	}
	root.Phases = append(root.Phases, r.bootstrapLeaderNode(servers, gravityPackage))
	for _, server := range servers[1:] {
		server := server
		root.Phases = append(root.Phases, r.bootstrapNode(server, gravityPackage))
	}
	return root
}

func (r *params) bootstrapLeaderNode(servers []storage.UpdateServer, gravityPackage loc.Locator) storage.OperationPhase {
	t := func(format string) string {
		return fmt.Sprintf(format, servers[0].Hostname)
	}
	return storage.OperationPhase{
		ID:          t("/bootstrap/%v"),
		Description: t("Bootstrap node %q"),
		Executor:    updateBootstrapLeader,
		Data: &storage.OperationPhaseData{
			ExecServer:       &servers[0].Server,
			Package:          &r.updateApp.Package,
			InstalledPackage: &r.installedApp.Package,
			Update: &storage.UpdateOperationData{
				Servers:        servers,
				GravityPackage: &gravityPackage,
			},
		},
	}
}

func (r *params) bootstrapNode(server storage.UpdateServer, gravityPackage loc.Locator) storage.OperationPhase {
	t := func(format string) string {
		return fmt.Sprintf(format, server.Hostname)
	}
	return storage.OperationPhase{
		ID:          t("/bootstrap/%v"),
		Description: t("Bootstrap node %q"),
		Executor:    updateBootstrap,
		Data: &storage.OperationPhaseData{
			ExecServer:       &server.Server,
			Package:          &r.updateApp.Package,
			InstalledPackage: &r.installedApp.Package,
			Update: &storage.UpdateOperationData{
				Servers:        []storage.UpdateServer{server},
				GravityPackage: &gravityPackage,
			},
		},
	}
}

func (r *params) bootstrapVersioned(servers []storage.UpdateServer, version string, gravityPackage loc.Locator, requires ...string) storage.OperationPhase {
	root := storage.OperationPhase{
		ID:            "/bootstrap",
		Description:   "Bootstrap update operation on nodes",
		Requires:      requires,
		LimitParallel: numParallelPhases,
	}
	root.Phases = append(root.Phases, r.bootstrapLeaderNodeVersioned(servers, version, gravityPackage))
	for _, server := range servers[1:] {
		server := server
		root.Phases = append(root.Phases, r.bootstrapNodeVersioned(server, version, gravityPackage))
	}
	return root
}

func (r *params) bootstrapLeaderNodeVersioned(servers []storage.UpdateServer, version string, gravityPackage loc.Locator) storage.OperationPhase {
	t := func(format string) string {
		return fmt.Sprintf(format, servers[0].Hostname)
	}
	return storage.OperationPhase{
		ID:          t("/bootstrap/%v"),
		Description: t("Bootstrap node %q"),
		Executor:    updateBootstrapLeader,
		Data: &storage.OperationPhaseData{
			ExecServer:       &servers[0].Server,
			Package:          &r.updateApp.Package,
			InstalledPackage: &r.installedApp.Package,
			Update: &storage.UpdateOperationData{
				Servers:           servers,
				RuntimeAppVersion: version,
				GravityPackage:    &gravityPackage,
			},
		},
	}
}

func (r *params) bootstrapNodeVersioned(server storage.UpdateServer, version string, gravityPackage loc.Locator) storage.OperationPhase {
	t := func(format string) string {
		return fmt.Sprintf(format, server.Hostname)
	}
	return storage.OperationPhase{
		ID:          t("bootstrap/%v"),
		Description: t("Bootstrap node %q"),
		Executor:    updateBootstrap,
		Data: &storage.OperationPhaseData{
			ExecServer:       &server.Server,
			Package:          &r.updateApp.Package,
			InstalledPackage: &r.installedApp.Package,
			Update: &storage.UpdateOperationData{
				Servers:           []storage.UpdateServer{server},
				RuntimeAppVersion: version,
				GravityPackage:    &gravityPackage,
			},
		},
	}
}

func (r params) etcd(leadMaster storage.Server, otherMasters []storage.UpdateServer, etcd etcdVersion) storage.OperationPhase {
	return storage.OperationPhase{
		ID:          "/etcd",
		Description: fmt.Sprintf("Upgrade etcd %v to %v", etcd.installed, etcd.update),
		Phases: []storage.OperationPhase{
			{
				ID:          "/etcd/backup",
				Description: "Backup etcd data",
				Phases: []storage.OperationPhase{
					r.etcdBackupNode(leadMaster),
					// FIXME: assumes len(otherMasters) == 1
					r.etcdBackupNode(otherMasters[0].Server),
				},
			},
			{
				ID:            "/etcd/shutdown",
				Description:   "Shutdown etcd cluster",
				LimitParallel: etcdNumParallel,
				Phases: []storage.OperationPhase{
					r.etcdShutdownNode(leadMaster, true),
					// FIXME: assumes len(otherMasters) == 1
					r.etcdShutdownNode(otherMasters[0].Server, false),
				},
			},
			{
				ID:            "/etcd/upgrade",
				Description:   "Upgrade etcd servers",
				LimitParallel: etcdNumParallel,
				Phases: []storage.OperationPhase{
					r.etcdUpgradeNode(leadMaster),
					// FIXME: assumes len(otherMasters) == 1
					r.etcdUpgradeNode(otherMasters[0].Server),
					// upgrade regular nodes
				},
			},
			{
				ID:          "/etcd/migrate",
				Description: "Migrate etcd data to new version",
				Phases: []storage.OperationPhase{
					r.etcdMigrateNode(leadMaster, etcd),
					// FIXME: assumes len(otherMasters) == 1
					r.etcdMigrateNode(otherMasters[0].Server, etcd),
				},
			},
			{
				ID:            "/etcd/restart",
				Description:   "Restart etcd servers",
				LimitParallel: etcdNumParallel,
				Phases: []storage.OperationPhase{
					r.etcdRestartLeaderNode(leadMaster),
					// FIXME: assumes len(otherMasters) == 1
					r.etcdRestartNode(otherMasters[0].Server),
					r.etcdRestartGravity(leadMaster),
				},
			},
		},
	}
}

func (r params) etcdBackupNode(server storage.Server) storage.OperationPhase {
	t := func(format string) string {
		return fmt.Sprintf(format, server.Hostname)
	}
	return storage.OperationPhase{
		ID:          t("/etcd/backup/%v"),
		Description: t("Backup etcd on node %q"),
		Executor:    updateEtcdBackup,
		Data: &storage.OperationPhaseData{
			Server: &server,
		},
	}
}

func (r params) etcdShutdownNode(server storage.Server, isLeader bool) storage.OperationPhase {
	t := func(format string) string {
		return fmt.Sprintf(format, server.Hostname)
	}
	return storage.OperationPhase{
		ID:          t("/etcd/shutdown/%v"),
		Description: t("Shutdown etcd on node %q"),
		Executor:    updateEtcdShutdown,
		Requires:    []string{t("/etcd/backup/%v")},
		Data: &storage.OperationPhaseData{
			Server: &server,
			Data:   strconv.FormatBool(isLeader),
		},
	}
}

func (r params) etcdUpgradeNode(server storage.Server) storage.OperationPhase {
	t := func(format string) string {
		return fmt.Sprintf(format, server.Hostname)
	}
	return storage.OperationPhase{
		ID:          t("/etcd/upgrade/%v"),
		Description: t("Upgrade etcd on node %q"),
		Executor:    updateEtcdMaster,
		Requires:    []string{t("/etcd/shutdown/%v")},
		Data: &storage.OperationPhaseData{
			Server: &server,
		},
	}
}

func (r params) etcdMigrateNode(server storage.Server, etcd etcdVersion) storage.OperationPhase {
	t := func(format string) string {
		return fmt.Sprintf(format, server.Hostname)
	}
	return storage.OperationPhase{
		ID: t("/etcd/migrate/%v"),
		Description: fmt.Sprintf("Migrate etcd data to version %v on node %q",
			etcd.update, server.Hostname),
		Executor: updateEtcdMigrate,
		Requires: []string{t("/etcd/upgrade/%v")},
		Data: &storage.OperationPhaseData{
			Server: &server,
			Update: &storage.UpdateOperationData{
				Etcd: &storage.EtcdUpgrade{
					From: etcd.installed,
					To:   etcd.update,
				},
			},
		},
	}
}

func (r params) etcdRestartLeaderNode(leadMaster storage.Server) storage.OperationPhase {
	t := func(format string) string {
		return fmt.Sprintf(format, leadMaster.Hostname)
	}
	return storage.OperationPhase{
		ID:          t("/etcd/restart/%v"),
		Description: t("Restart etcd on node %q"),
		Executor:    updateEtcdRestart,
		Requires:    []string{t("/etcd/migrate/%v")},
		Data: &storage.OperationPhaseData{
			Server: &leadMaster,
		},
	}
}

func (r params) etcdRestartNode(server storage.Server) storage.OperationPhase {
	t := func(format string) string {
		return fmt.Sprintf(format, server.Hostname)
	}
	return storage.OperationPhase{
		ID:          t("/etcd/restart/%v"),
		Description: t("Restart etcd on node %q"),
		Executor:    updateEtcdRestart,
		Requires:    []string{t("/etcd/migrate/%v")},
		Data: &storage.OperationPhaseData{
			Server: &server,
		},
	}
}

func (r params) etcdRestartGravity(leadMaster storage.Server) storage.OperationPhase {
	return storage.OperationPhase{
		ID:          fmt.Sprint("/etcd/restart/", constants.GravityServiceName),
		Description: fmt.Sprint("Restart ", constants.GravityServiceName, " service"),
		Executor:    updateEtcdRestartGravity,
		Data: &storage.OperationPhaseData{
			Server: &leadMaster,
		},
	}
}

func (r *params) migration(requires ...string) storage.OperationPhase {
	phase := storage.OperationPhase{
		ID:          "/migration",
		Description: "Perform system database migration",
		Requires:    requires,
	}
	if len(r.links) != 0 && len(r.trustedClusters) == 0 {
		phase.Phases = append(phase.Phases, storage.OperationPhase{
			ID:          "/migration/links",
			Description: "Migrate remote Gravity Hub links to trusted clusters",
			Executor:    migrateLinks,
		})
	}
	phase.Phases = append(phase.Phases, storage.OperationPhase{
		ID:          "/migration/labels",
		Description: "Update node labels",
		Executor:    updateLabels,
	})
	// FIXME: add roles migration step
	return phase
}

func (r params) config(requires ...string) storage.OperationPhase {
	masters, _ := fsm.SplitServers(r.servers)
	masters = reorderStorageServers(masters, *r.planConfig.leadMaster)
	return storage.OperationPhase{
		ID:            "/config",
		Description:   "Update system configuration on nodes",
		Requires:      requires,
		LimitParallel: numParallelPhases,
		Phases: []storage.OperationPhase{
			r.configNode(masters[0]),
			r.configNode(masters[1]),
		},
	}
}

func (r params) configNode(server storage.Server) storage.OperationPhase {
	t := func(format string) string {
		return fmt.Sprintf(format, server.Hostname)
	}
	return storage.OperationPhase{
		ID:          t("/config/%v"),
		Executor:    config,
		Description: t("Update system configuration on node %q"),
		Data: &storage.OperationPhaseData{
			Server: &server,
		},
	}
}

func (r params) runtime(updates []loc.Locator, requires ...string) storage.OperationPhase {
	phase := storage.OperationPhase{
		ID:          "/runtime",
		Description: "Update application runtime",
		Requires:    requires,
	}
	var deps []string
	for _, update := range updates {
		app := runtimeUpdate(update, deps...)
		phase.Phases = append(phase.Phases, app)
		deps = []string{app.ID}
	}
	return phase
}

func runtimeUpdate(loc loc.Locator, requires ...string) storage.OperationPhase {
	return storage.OperationPhase{
		ID:          fmt.Sprintf("/runtime/%v", loc.Name),
		Executor:    updateApp,
		Description: fmt.Sprintf("Update system application %q to %v", loc.Name, loc.Version),
		Data: &storage.OperationPhaseData{
			Package: &loc,
		},
		Requires: requires,
	}
}

func (r params) app(requires ...string) storage.OperationPhase {
	phase := storage.OperationPhase{
		ID:          "/app",
		Description: "Update installed application",
		Requires:    requires,
	}
	for _, update := range r.appUpdates {
		phase.Phases = append(phase.Phases, appUpdate(update))
	}
	return phase
}

func appUpdate(loc loc.Locator, requires ...string) storage.OperationPhase {
	return storage.OperationPhase{
		ID:          fmt.Sprintf("/app/%v", loc.Name),
		Executor:    updateApp,
		Description: fmt.Sprintf("Update application %q to %v", loc.Name, loc.Version),
		Data: &storage.OperationPhaseData{
			Package: &loc,
		},
		Requires: requires,
	}
}

func (r params) cleanup() storage.OperationPhase {
	return storage.OperationPhase{
		ID:            "/gc",
		Description:   "Run cleanup tasks",
		Requires:      []string{"/app"},
		LimitParallel: numParallelPhases,
		Phases: []storage.OperationPhase{
			{
				ID:          "/gc/node-1",
				Executor:    cleanupNode,
				Description: `Clean up node "node-1"`,
				Data: &storage.OperationPhaseData{
					Server: &r.servers[0],
				},
			},
			{
				ID:          "/gc/node-2",
				Executor:    cleanupNode,
				Description: `Clean up node "node-2"`,
				Data: &storage.OperationPhaseData{
					Server: &r.servers[1],
				},
			},
			{
				ID:          "/gc/node-3",
				Executor:    cleanupNode,
				Description: `Clean up node "node-3"`,
				Data: &storage.OperationPhaseData{
					Server: &r.servers[2],
				},
			},
		},
	}
}

func (r params) updates() []storage.UpdateServer {
	result := make([]storage.UpdateServer, 0, len(r.servers))
	for _, s := range r.servers {
		result = append(result, r.newUpdateServer(s))
	}
	return result
}

type params struct {
	// configuration
	planConfig

	installedRuntimeApp app.Application
	installedApp        app.Application
	updateRuntimeApp    app.Application
	updateApp           app.Application
	teleportLoc         loc.Locator

	// expectations
	gravityPackage loc.Locator
	etcdVersion    etcdVersion
	runtimeUpdates []loc.Locator
	appUpdates     []loc.Locator
}

func (r testRotator) RotateSecrets(ops.RotateSecretsRequest) (*ops.RotatePackageResponse, error) {
	return &ops.RotatePackageResponse{Locator: r.secretsPackage}, nil
}

func (r testRotator) RotatePlanetConfig(ops.RotatePlanetConfigRequest) (*ops.RotatePackageResponse, error) {
	return &ops.RotatePackageResponse{Locator: r.runtimeConfigPackage}, nil
}

func (r testRotator) RotateTeleportConfig(ops.RotateTeleportConfigRequest) (*ops.RotatePackageResponse, *ops.RotatePackageResponse, error) {
	return &ops.RotatePackageResponse{Locator: r.teleportMasterPackage},
		&ops.RotatePackageResponse{Locator: r.teleportNodePackage},
		nil
}

var testOperator = testRotator{
	secretsPackage:        newLoc("secrets:0.0.1"),
	runtimeConfigPackage:  newLoc("planet-config:0.0.1"),
	teleportMasterPackage: newLoc("teleport-master-config:0.0.1"),
	teleportNodePackage:   newLoc("teleport-node-config:0.0.1"),
}

type testRotator struct {
	secretsPackage        loc.Locator
	runtimeConfigPackage  loc.Locator
	teleportMasterPackage loc.Locator
	teleportNodePackage   loc.Locator
}

func mustLocator(loc *loc.Locator, err error) loc.Locator {
	if err != nil {
		panic(err)
	}
	return *loc
}

func reorderStorageServers(servers []storage.Server, server storage.Server) (result []storage.Server) {
	sort.Slice(servers, func(i, j int) bool {
		// Push server to the front
		return servers[i].AdvertiseIP == server.AdvertiseIP
	})
	return servers
}

func newApp(appLoc string, manifestBytes string) app.Application {
	return app.Application{
		Package:  loc.MustParseLocator(appLoc),
		Manifest: schema.MustParseManifestYAML([]byte(manifestBytes)),
		PackageEnvelope: pack.PackageEnvelope{
			Manifest: []byte(manifestBytes),
		},
	}
}

func newWorker(name string) storage.Server {
	return storage.Server{
		AdvertiseIP: name,
		Hostname:    name,
		Role:        "node",
		ClusterRole: string(schema.ServiceRoleNode),
	}
}

func newMaster(name string) storage.Server {
	return storage.Server{
		AdvertiseIP: name,
		Hostname:    name,
		Role:        "node",
		ClusterRole: string(schema.ServiceRoleMaster),
	}
}

func (r params) newUpdateServer(s storage.Server) storage.UpdateServer {
	return storage.UpdateServer{
		Server: s,
		Runtime: storage.RuntimePackage{
			Installed:      r.installedRuntimeApp.Manifest.SystemOptions.Dependencies.Runtime.Locator,
			SecretsPackage: &testOperator.secretsPackage,
			Update: &storage.RuntimeUpdate{
				Package:       r.updateRuntimeApp.Manifest.SystemOptions.Dependencies.Runtime.Locator,
				ConfigPackage: testOperator.runtimeConfigPackage,
			},
		},
		Teleport: storage.TeleportPackage{
			Installed: r.teleportLoc,
		},
	}
}

func newVer(v string) semver.Version {
	return *semver.New(v)
}

func newLoc(nameVersion string) loc.Locator {
	parts := strings.Split(nameVersion, ":")
	if len(parts) != 2 {
		panic("invalid package reference")
	}
	return loc.Locator{
		Repository: defaults.SystemAccountOrg,
		Name:       parts[0],
		Version:    parts[1],
	}
}

func newPackage(nameVersion string) apptest.Package {
	return apptest.Package{
		Loc: newLoc(nameVersion),
	}
}

func newRuntimePackageWithEtcd(loc loc.Locator, etcdVersion string) apptest.Package {
	return apptest.Package{
		Loc: loc,
		Items: []*archive.Item{
			archive.ItemFromString("orbit.manifest.json", fmt.Sprintf(`{
	"version": "0.0.1",
	"labels": [
		{
			"name": "version-etcd",
			"value": "v%s"
		}
	]
}`, etcdVersion)),
		},
	}
}

func testIDs(id int) idGen {
	return func() string {
		newID := id
		id++
		return fmt.Sprint(newID)
	}
}

const (
	numParallelPhases  = 3
	numParallelWorkers = 4
)

var (
	runtimePackage = storage.RuntimePackage{
		Update: &storage.RuntimeUpdate{
			Package: loc.MustParseLocator("gravitational.io/planet:2.0.0"),
		},
	}
	intermediateRuntimePackage = storage.RuntimePackage{
		Update: &storage.RuntimeUpdate{
			Package: loc.MustParseLocator("gravitational.io/planet:1.2.0"),
		},
	}
	gravityInstalledLoc = loc.MustParseLocator("gravitational.io/gravity:1.0.0")
	serviceUser         = storage.OSUser{Name: "user", UID: "1000", GID: "1000"}
	userConfig          = UserConfig{ParallelWorkers: numParallelWorkers}
)

const (
	installedRuntimeAppManifest = `apiVersion: bundle.gravitational.io/v2
kind: Runtime
metadata:
  name: runtime
  resourceVersion: 1.0.0
dependencies:
  packages:
    - gravitational.io/gravity:1.0.0
  apps:
    - gravitational.io/runtime-dep-1:1.0.0
    - gravitational.io/runtime-dep-2:1.0.0
    - gravitational.io/rbac-app:1.0.0
`

	intermediateRuntimeAppManifest = `apiVersion: bundle.gravitational.io/v2
kind: Runtime
metadata:
  name: runtime
  resourceVersion: 2.0.0
dependencies:
  packages:
    - gravitational.io/gravity:2.0.0
  apps:
    - gravitational.io/runtime-dep-1:1.0.0
    - gravitational.io/runtime-dep-2:2.0.0
    - gravitational.io/rbac-app:2.0.0
`

	updateRuntimeAppManifest = `apiVersion: bundle.gravitational.io/v2
kind: Runtime
metadata:
  name: runtime
  resourceVersion: 3.0.0
dependencies:
  packages:
    - gravitational.io/gravity:3.0.0
  apps:
    - gravitational.io/runtime-dep-1:1.0.0
    - gravitational.io/runtime-dep-2:3.0.0
    - gravitational.io/rbac-app:3.0.0
`

	installedAppManifest = `apiVersion: bundle.gravitational.io/v2
kind: Bundle
metadata:
  name: app
  resourceVersion: 1.0.0
dependencies:
  apps:
    - gravitational.io/app-dep-1:1.0.0
    - gravitational.io/app-dep-2:1.0.0
nodeProfiles:
  - name: node
systemOptions:
  dependencies:
    runtimePackage: gravitational.io/planet:1.0.0
`

	updateAppManifest = `apiVersion: bundle.gravitational.io/v2
kind: Bundle
metadata:
  name: app
  resourceVersion: 3.0.0
dependencies:
  apps:
    - gravitational.io/app-dep-1:1.0.0
    - gravitational.io/app-dep-2:3.0.0
nodeProfiles:
  - name: node
systemOptions:
  dependencies:
    runtimePackage: gravitational.io/planet:3.0.0
`
)
