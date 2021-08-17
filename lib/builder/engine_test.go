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

package builder

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	dockerarchive "github.com/docker/docker/pkg/archive"
	libapp "github.com/gravitational/gravity/lib/app"
	app "github.com/gravitational/gravity/lib/app/service"
	apptest "github.com/gravitational/gravity/lib/app/service/test"
	"github.com/gravitational/gravity/lib/archive"
	"github.com/gravitational/gravity/lib/defaults"
	"github.com/gravitational/gravity/lib/docker"
	"github.com/gravitational/gravity/lib/loc"
	"github.com/gravitational/gravity/lib/localenv"
	"github.com/gravitational/gravity/lib/pack"
	packtest "github.com/gravitational/gravity/lib/pack/test"
	"github.com/gravitational/gravity/lib/schema"
	"github.com/gravitational/gravity/lib/utils"

	"github.com/coreos/go-semver/semver"
	"github.com/gravitational/trace"
	"github.com/gravitational/version"
	check "gopkg.in/check.v1"
)

func TestBuilder(t *testing.T) { check.TestingT(t) }

type BuilderSuite struct{}

var _ = check.Suite(&BuilderSuite{})

type InstallerBuilderSuite struct{}

var _ = check.Suite(&InstallerBuilderSuite{})

type CustomImageBuilderSuite struct{}

var _ = check.Suite(&CustomImageBuilderSuite{})

func (s *BuilderSuite) TestVersionsCompatibility(c *check.C) {
	testCases := []struct {
		teleVer    semver.Version
		runtimeVer semver.Version
		compatible bool
		comment    string
	}{
		{
			teleVer:    *semver.New("5.5.0"),
			runtimeVer: *semver.New("5.5.0"),
			compatible: true,
			comment:    "tele and runtime versions are the same",
		},
		{
			teleVer:    *semver.New("5.5.1"),
			runtimeVer: *semver.New("5.5.0"),
			compatible: true,
			comment:    "tele version is newer than runtime version",
		},
		{
			teleVer:    *semver.New("5.5.0"),
			runtimeVer: *semver.New("5.5.1"),
			compatible: false,
			comment:    "runtime version is newer than tele version",
		},
		{
			teleVer:    *semver.New("5.5.0"),
			runtimeVer: *semver.New("5.4.0"),
			compatible: false,
			comment:    "tele and runtime versions are different releases",
		},
	}
	for _, t := range testCases {
		c.Assert(versionsCompatible(t.teleVer, t.runtimeVer), check.Equals,
			t.compatible, check.Commentf(t.comment))
	}
}

func (s *BuilderSuite) TestSelectRuntimeVersion(c *check.C) {
	b := &Engine{
		Config: Config{
			Progress: utils.DiscardProgress,
		},
	}

	manifest := schema.MustParseManifestYAML([]byte(manifestWithBase))
	ver, err := b.SelectRuntime(&manifest)
	c.Assert(err, check.IsNil)
	c.Assert(ver, check.DeepEquals, semver.New("5.5.0"))

	manifest = schema.MustParseManifestYAML([]byte(manifestWithoutBase))
	version.Init("5.4.2")
	ver, err = b.SelectRuntime(&manifest)
	c.Assert(err, check.IsNil)
	c.Assert(ver, check.DeepEquals, semver.New("5.4.2"))

	manifest = schema.MustParseManifestYAML([]byte(manifestInvalidBase))
	_, err = b.SelectRuntime(&manifest)
	c.Assert(err, check.FitsTypeOf, trace.BadParameter(""))
	c.Assert(err, check.ErrorMatches, "unsupported base image .*")
}

func (s *InstallerBuilderSuite) TestBuildInstallerWithDefaultPlanetPackage(c *check.C) {
	// client := docker.CheckDocker(c)

	// 	var (
	// 		manifestBytes = []byte(`
	// apiVersion: cluster.gravitational.io/v2
	// kind: Cluster
	// metadata:
	//   name: app
	//   resourceVersion: "0.0.1"
	// dependencies:
	//   apps:
	//     - gravitational.io/app-dependency:0.0.1
	// installer:
	//   flavors:
	//     items:
	//       - name: "one"
	//         nodes:
	//           - profile: master
	//             count: 1
	//       - name: "three"
	//         nodes:
	//           - profile: master
	//             count: 1
	//           - profile: node
	//             count: 2
	// nodeProfiles:
	//   - name: master
	//     labels:
	//       node-role.kubernetes.io/master: "true"
	//   - name: node
	//     labels:
	//       node-role.kubernetes.io/node: "true"
	// systemOptions:
	//   runtime:
	//     version: 0.0.1
	// `)
	//
	// 		dependencyManifestBytes = []byte(`
	// apiVersion: bundle.gravitational.io/v2
	// kind: SystemApplication
	// metadata:
	//   name: app-dependency
	//   resourceVersion: "0.0.1"
	// `)
	// 	)

	// setup
	appLoc := loc.MustParseLocator("gravitational.io/app:0.0.1")
	remoteEnv := newEnviron(c)
	defer remoteEnv.Close()
	buildEnv := newEnviron(c)
	defer buildEnv.Close()
	appDir := c.MkDir()
	manifestPath := filepath.Join(appDir, defaults.ManifestFileName)
	app := apptest.CreateApplication(apptest.AppRequest{
		App:      apptest.DefaultClusterApplication(appLoc).Build(),
		Apps:     remoteEnv.Apps,
		Packages: remoteEnv.Packages,
	}, c)
	//writeFile(manifestPath, manifestBytes, c)
	manifestBytes, err := json.Marshal(app.Manifest)
	c.Assert(err, check.IsNil)
	writeFile(manifestPath, manifestBytes, c)

	// FIXME(dima)
	// createRuntimeApplication(remoteEnv, c)
	// createApp(dependencyManifestBytes, remoteEnv.Apps, c)
	// createApp(manifestBytes, remoteEnv.Apps, c)

	b, err := NewClusterBuilder(Config{
		Progress:         utils.DiscardProgress,
		Env:              buildEnv,
		SkipVersionCheck: true,
		Repository:       "test-repository",
		Syncer:           NewPackSyncer(remoteEnv.Packages, remoteEnv.Apps, "test-repository"),
	})
	c.Assert(err, check.IsNil)

	// verify
	err = b.Build(context.TODO(), ClusterRequest{
		OutputPath: filepath.Join(buildEnv.StateDir, "app.tar"),
		//ManifestPath: manifestPath,
		SourcePath: filepath.Dir(manifestPath),
	})
	c.Assert(err, check.IsNil)
}

func (s *InstallerBuilderSuite) TestBuildInstallerWithMixedVersionPackages(c *check.C) {
	if !checkDockerAvailable() {
		c.Skip("test requires docker")
	}

	var (
		manifestBytes = []byte(`
apiVersion: cluster.gravitational.io/v2
kind: Cluster
metadata:
  name: app
  resourceVersion: "0.0.1"
installer:
  flavors:
    items:
      - name: "one"
        nodes:
          - profile: master
            count: 1
      - name: "three"
        nodes:
          - profile: master
            count: 1
          - profile: node
            count: 2
nodeProfiles:
  - name: master
    labels:
      node-role.kubernetes.io/master: "true"
  - name: node
    labels:
      node-role.kubernetes.io/node: "true"
systemOptions:
  runtime:
    version: 0.0.0+latest # use meta version as a placeholder
`)
	)

	// setup
	remoteEnv := newEnviron(c)
	defer remoteEnv.Close()
	buildEnv := newEnviron(c)
	defer buildEnv.Close()
	appDir := c.MkDir()
	manifestPath := filepath.Join(appDir, defaults.ManifestFileName)

	version.Init("0.0.1")
	writeFile(manifestPath, manifestBytes, c)
	outputPath := filepath.Join(buildEnv.StateDir, "app.tar")
	b, err := NewClusterBuilder(Config{
		// FieldLogger:      logrus.WithField(trace.Component, "test"),
		Progress:         utils.DiscardProgress,
		Env:              buildEnv,
		SkipVersionCheck: true,
		Repository:       "repository",
		Syncer:           NewPackSyncer(remoteEnv.Packages, remoteEnv.Apps, "repository"),
	})
	c.Assert(err, check.IsNil)

	// Simulate a workflow with packages/applications of mixed versions
	// and at least one version available above the requested runtime version
	// TODO(dima): use apptest builder APIs
	// createRuntimeApplicationWithVersion(buildEnv, "0.0.1", c)
	// createRuntimeApplicationWithVersion(buildEnv, "0.0.2", c)
	// createApp(manifestBytes, buildEnv.Apps, c)

	// verify
	err = b.Build(context.TODO(), ClusterRequest{
		OutputPath: outputPath,
		//ManifestPath:     manifestPath,
		SourcePath: filepath.Dir(manifestPath),
	})
	c.Assert(err, check.IsNil)

	unpackDir := c.MkDir()
	tarballEnv := unpackTarball(outputPath, unpackDir, c)
	defer tarballEnv.Close()

	packtest.VerifyPackages(tarballEnv.Packages, mustLoc(
		"gravitational.io/planet:0.0.1",
		"gravitational.io/app:0.0.1",
		"gravitational.io/gravity:0.0.1",
		"gravitational.io/kubernetes:0.0.1"), c)
}

func (s *InstallerBuilderSuite) TestBuildInstallerWithPackagesInCache(c *check.C) {
	if !checkDockerAvailable() {
		c.Skip("test requires docker")
	}

	var (
		manifestBytes = []byte(`
apiVersion: cluster.gravitational.io/v2
kind: Cluster
metadata:
  name: app
  resourceVersion: "0.0.1"
installer:
  flavors:
    items:
      - name: "one"
        nodes:
          - profile: master
            count: 1
      - name: "three"
        nodes:
          - profile: master
            count: 1
          - profile: node
            count: 2
nodeProfiles:
  - name: master
    labels:
      node-role.kubernetes.io/master: "true"
  - name: node
    labels:
      node-role.kubernetes.io/node: "true"
systemOptions:
  runtime:
    version: 0.0.2-dev.1
`)
	)

	// setup
	remoteEnv := newEnviron(c)
	defer remoteEnv.Close()
	buildEnv := newEnviron(c)
	defer buildEnv.Close()
	appDir := c.MkDir()
	manifestPath := filepath.Join(appDir, defaults.ManifestFileName)

	writeFile(manifestPath, manifestBytes, c)
	b, err := NewClusterBuilder(Config{
		// FieldLogger:      logrus.WithField(trace.Component, "test"),
		Progress:         utils.DiscardProgress,
		Env:              buildEnv,
		SkipVersionCheck: true,
		Repository:       "repository",
		Syncer:           NewPackSyncer(remoteEnv.Packages, remoteEnv.Apps, "repository"),
	})
	c.Assert(err, check.IsNil)

	// Simulate a development workflow with packages/applications
	// explicitly cached (but also unavailable in the remote hub)
	// TODO(dima): use apptest builder APIs
	// createRuntimeApplicationWithVersion(buildEnv, "0.0.2-dev.1", c)
	// createApp(manifestBytes, buildEnv.Apps, c)

	// verify
	err = b.Build(context.TODO(), ClusterRequest{
		OutputPath: filepath.Join(buildEnv.StateDir, "app.tar"),
		//ManifestPath:     manifestPath,
		SourcePath: filepath.Dir(manifestPath),
	})
	c.Assert(err, check.IsNil)
}

func (s *InstallerBuilderSuite) TestBuildInstallerWithIntermediateHops(c *check.C) {
	if !checkDockerAvailable() {
		c.Skip("test requires docker")
	}

	var (
		manifestBytes = []byte(`
apiVersion: cluster.gravitational.io/v2
kind: Cluster
metadata:
  name: app
  resourceVersion: "0.0.1"
installer:
  flavors:
    items:
      - name: "one"
        nodes:
          - profile: master
            count: 1
      - name: "three"
        nodes:
          - profile: master
            count: 1
          - profile: node
            count: 2
nodeProfiles:
  - name: master
    labels:
      node-role.kubernetes.io/master: "true"
  - name: node
    labels:
      node-role.kubernetes.io/node: "true"
systemOptions:
  runtime:
    version: 0.0.2
`)
	)

	// setup
	remoteEnv := newEnviron(c)
	defer remoteEnv.Close()
	buildEnv := newEnviron(c)
	defer buildEnv.Close()
	appDir := c.MkDir()
	manifestPath := filepath.Join(appDir, defaults.ManifestFileName)

	writeFile(manifestPath, manifestBytes, c)
	outputPath := filepath.Join(buildEnv.StateDir, "app.tar")
	b, err := NewClusterBuilder(Config{
		//FieldLogger:      logrus.WithField(trace.Component, "test"),
		Progress:         utils.DiscardProgress,
		Env:              buildEnv,
		SkipVersionCheck: true,
		Repository:       "repository",
		Syncer:           NewPackSyncer(remoteEnv.Packages, remoteEnv.Apps, "repository"),
		UpgradeVia:       []string{"0.0.1"},
	})
	c.Assert(err, check.IsNil)

	// TODO(dima): use apptest builder APIs
	// createRuntimeApplicationWithVersion(remoteEnv, "0.0.1", c)
	// createRuntimeApplicationWithVersion(remoteEnv, "0.0.2", c)
	// createApp(manifestBytes, remoteEnv.Apps, c)

	// verify
	err = b.Build(context.TODO(), ClusterRequest{
		OutputPath: outputPath,
		SourcePath: filepath.Dir(manifestPath),
	})
	c.Assert(err, check.IsNil)

	unpackDir := c.MkDir()
	tarballEnv := unpackTarball(outputPath, unpackDir, c)
	defer tarballEnv.Close()

	packtest.VerifyPackagesWithLabels(tarballEnv.Packages, packtest.PackagesWithLabels{
		packtest.NewPackage("gravitational.io/planet:0.0.2"),
		packtest.NewPackage("gravitational.io/planet:0.0.1",
			pack.PurposeRuntimeUpgrade, "0.0.1",
		),
		packtest.NewPackage("gravitational.io/app:0.0.1"),
		packtest.NewPackage("gravitational.io/gravity:0.0.1",
			pack.PurposeRuntimeUpgrade, "0.0.1",
		),
		packtest.NewPackage("gravitational.io/gravity:0.0.2"),
		packtest.NewPackage("gravitational.io/kubernetes:0.0.1",
			pack.PurposeRuntimeUpgrade, "0.0.1",
		),
		packtest.NewPackage("gravitational.io/kubernetes:0.0.2"),
	}, c)
}

func (s *BuilderSuite) TestBuildInstallerWithDefaultPlanetPackageFromLegacyHub(c *check.C) {
	if !checkDockerAvailable() {
		c.Skip("test requires docker")
	}
	// setup
	remoteEnv := newEnviron(c)
	defer remoteEnv.Close()
	buildEnv := newEnviron(c)
	defer buildEnv.Close()
	appDir := c.MkDir()
	manifestPath := filepath.Join(appDir, defaults.ManifestFileName)
	manifestBytes := []byte(`
apiVersion: cluster.gravitational.io/v2
kind: Cluster
metadata:
  name: app
  resourceVersion: "0.0.1"
installer:
  flavors:
    items:
      - name: "one"
        nodes:
          - profile: master
            count: 1
      - name: "three"
        nodes:
          - profile: master
            count: 1
          - profile: node
            count: 2
nodeProfiles:
  - name: master
    labels:
      node-role.kubernetes.io/master: "true"
  - name: node
    labels:
      node-role.kubernetes.io/node: "true"
systemOptions:
  #dependencies:
  #  runtimePackage: gravitational.io/planet:0.0.1
  runtime:
    version: 0.0.1
`)
	writeFile(manifestPath, manifestBytes, c)
	b, err := NewClusterBuilder(Config{
		//FieldLogger:      logrus.WithField(trace.Component, "test"),
		Progress:         utils.DiscardProgress,
		Env:              buildEnv,
		SkipVersionCheck: true,
		Repository:       "repository",
		Syncer:           NewPackSyncer(remoteEnv.Packages, newHubApps(remoteEnv.Apps), "repository"),
	})
	c.Assert(err, check.IsNil)

	//
	// TODO(dima): use apptest builder APIs
	// createRuntimeApplication(remoteEnv, c)
	// createApp(manifestBytes, remoteEnv.Apps, c)

	// verify
	err = b.Build(context.TODO(), ClusterRequest{
		OutputPath: filepath.Join(buildEnv.StateDir, "app.tar"),
		SourcePath: filepath.Dir(manifestPath),
	})
	c.Assert(err, check.IsNil)
}

func (s *CustomImageBuilderSuite) SetUpTest(c *check.C) {
	_ = docker.CheckDocker(c)
	// FIXME(dima): use dockertest builder API
	// if !checkDockerAvailable() {
	// 	c.Skip("test requires docker")
	// }
	// dockerDir := c.MkDir()
	// createPlanetDockerImage(dockerDir, planetTag, c)
}

func (s *CustomImageBuilderSuite) TearDownTest(c *check.C) {
	// FIXME(dima): use dockertest builder API
	// removePlanetDockerImage(c)
}

func (s *CustomImageBuilderSuite) TestBuildInstallerWithCustomGlobalPlanetPackage(c *check.C) {
	remoteEnv := newEnviron(c)
	defer remoteEnv.Close()
	buildEnv := newEnviron(c)
	defer buildEnv.Close()
	appDir := c.MkDir()
	manifestPath := filepath.Join(appDir, defaults.ManifestFileName)
	manifestBytes := []byte(`
apiVersion: cluster.gravitational.io/v2
kind: Cluster
metadata:
  name: app
  resourceVersion: "0.0.1"
installer:
  flavors:
    items:
      - name: "one"
        nodes:
          - profile: master
            count: 1
      - name: "three"
        nodes:
          - profile: master
            count: 1
          - profile: node
            count: 2
nodeProfiles:
  - name: master
    labels:
      node-role.kubernetes.io/master: "true"
  - name: node
    labels:
      node-role.kubernetes.io/node: "true"
systemOptions:
  baseImage: quay.io/gravitational/planet:0.0.2
  runtime:
    version: 0.0.1
`)
	writeFile(manifestPath, manifestBytes, c)
	outputPath := filepath.Join(buildEnv.StateDir, "app.tar")
	b, err := NewClusterBuilder(Config{
		//FieldLogger:      logrus.WithField(trace.Component, "test"),
		Progress:         utils.DiscardProgress,
		Env:              buildEnv,
		SkipVersionCheck: true,
		Repository:       "repository",
		Syncer:           NewPackSyncer(remoteEnv.Packages, newHubApps(remoteEnv.Apps), "repository"),
	})
	c.Assert(err, check.IsNil)

	// TODO(dima): use apptest builder APIs
	// createRuntimeApplication(remoteEnv, c)
	// createApp(manifestBytes, remoteEnv.Apps, c)

	// verify
	err = b.Build(context.TODO(), ClusterRequest{
		OutputPath: outputPath,
		SourcePath: filepath.Dir(manifestPath),
		Vendor: app.VendorRequest{
			PackageName:      "app",
			PackageVersion:   "0.0.1",
			VendorRuntime:    true,
			ManifestPath:     manifestPath,
			ResourcePatterns: []string{defaults.VendorPattern},
		},
	})
	c.Assert(err, check.IsNil)

	unpackDir := c.MkDir()
	tarballEnv := unpackTarball(outputPath, unpackDir, c)
	defer tarballEnv.Close()

	packtest.VerifyPackages(tarballEnv.Packages, mustLoc(
		"gravitational.io/planet:0.0.2",
		"gravitational.io/app:0.0.1",
		"gravitational.io/gravity:0.0.1",
		"gravitational.io/kubernetes:0.0.1"), c)
}

func (s *CustomImageBuilderSuite) TestBuildInstallerWithCustomPerNodePlanetPackage(c *check.C) {
	remoteEnv := newEnviron(c)
	defer remoteEnv.Close()
	buildEnv := newEnviron(c)
	defer buildEnv.Close()
	appDir := c.MkDir()
	manifestPath := filepath.Join(appDir, defaults.ManifestFileName)
	manifestBytes := []byte(`
apiVersion: cluster.gravitational.io/v2
kind: Cluster
metadata:
  name: app
  resourceVersion: "0.0.1"
installer:
  flavors:
    items:
      - name: "one"
        nodes:
          - profile: master
            count: 1
      - name: "three"
        nodes:
          - profile: master
            count: 1
          - profile: node
            count: 2
nodeProfiles:
  - name: master
    labels:
      node-role.kubernetes.io/master: "true"
  - name: node
    systemOptions:
      baseImage: quay.io/gravitational/planet:0.0.2
    labels:
      node-role.kubernetes.io/node: "true"
systemOptions:
  runtime:
    version: 0.0.1
`)
	writeFile(manifestPath, manifestBytes, c)
	outputPath := filepath.Join(buildEnv.StateDir, "app.tar")
	b, err := NewClusterBuilder(Config{
		//FieldLogger:      logrus.WithField(trace.Component, "test"),
		Progress:         utils.DiscardProgress,
		Env:              buildEnv,
		SkipVersionCheck: true,
		Repository:       "repository",
		Syncer:           NewPackSyncer(remoteEnv.Packages, newHubApps(remoteEnv.Apps), "repository"),
	})
	c.Assert(err, check.IsNil)

	// TODO(dima): use apptest builder APIs
	// createRuntimeApplication(remoteEnv, c)
	// createApp(manifestBytes, remoteEnv.Apps, c)

	// verify
	err = b.Build(context.TODO(), ClusterRequest{
		OutputPath: outputPath,
		SourcePath: manifestPath,
		Vendor: app.VendorRequest{
			PackageName:      "app",
			PackageVersion:   "0.0.1",
			VendorRuntime:    true,
			ManifestPath:     manifestPath,
			ResourcePatterns: []string{defaults.VendorPattern},
		},
	})
	c.Assert(err, check.IsNil)

	unpackDir := c.MkDir()
	tarballEnv := unpackTarball(outputPath, unpackDir, c)
	defer tarballEnv.Close()

	packtest.VerifyPackages(tarballEnv.Packages, mustLoc(
		// Per-node custom planet configuration adds to the list of planet packages
		"gravitational.io/quay.io-gravitational-planet:0.0.2",
		"gravitational.io/planet:0.0.1",
		"gravitational.io/app:0.0.1",
		"gravitational.io/gravity:0.0.1",
		"gravitational.io/kubernetes:0.0.1"), c)
}

func mustLoc(pkgs ...string) (result []loc.Locator) {
	for _, pkg := range pkgs {
		result = append(result, loc.MustParseLocator(pkg))
	}
	return result
}

func newEnviron(c *check.C) *localenv.LocalEnvironment {
	env, err := localenv.New(c.MkDir())
	c.Assert(err, check.IsNil)
	c.Assert(env.Packages.UpsertRepository(defaults.SystemAccountOrg, time.Time{}), check.IsNil)
	return env
}

// TODO(dima): remove me
// func createRuntimeApplication(env *localenv.LocalEnvironment, c *check.C) {
// 	createRuntimeApplicationWithVersion(env, "0.0.1", c)
// }
//
// func createRuntimeApplicationWithVersion(env *localenv.LocalEnvironment, version string, c *check.C) {
// 	runtimePackage := loc.Planet.WithLiteralVersion(version)
// 	gravityPackage := loc.Gravity.WithLiteralVersion(version)
// 	items := []*archive.Item{
// 		archive.ItemFromString("planet", "planet"),
// 	}
// 	apptest.CreatePackage(env.Packages, runtimePackage, items, c)
// 	items = []*archive.Item{
// 		archive.ItemFromString("gravity", "gravity"),
// 	}
// 	apptest.CreatePackage(env.Packages, gravityPackage, items, c)
// 	manifestBytes := fmt.Sprintf(`apiVersion: bundle.gravitational.io/v2
// kind: Runtime
// metadata:
//   name: kubernetes
//   resourceVersion: %[1]v
// dependencies:
//   packages:
//   - gravitational.io/planet:%[1]v
//   - gravitational.io/gravity:%[1]v
// systemOptions:
//   dependencies:
//     runtimePackage: gravitational.io/planet:%[1]v
// `, version)
// 	runtimeAppLoc := loc.Runtime.WithLiteralVersion(version)
// 	items = []*archive.Item{
// 		archive.DirItem("resources"),
// 		archive.ItemFromString("resources/app.yaml", manifestBytes),
// 	}
// 	apptest.CreateApplicationFromData(env.Apps, runtimeAppLoc, items, c)
// }

// func createApp(manifestBytes []byte, apps libapp.Applications, c *check.C) *libapp.Application {
// 	manifest := schema.MustParseManifestYAML(manifestBytes)
// 	loc := loc.Locator{
// 		Repository: defaults.SystemAccountOrg,
// 		Name:       manifest.Metadata.Name,
// 		Version:    manifest.Metadata.ResourceVersion,
// 	}
// 	files := []*archive.Item{
// 		archive.DirItem("resources"),
// 		archive.ItemFromString("resources/app.yaml", string(manifestBytes)),
// 	}
// 	return apptest.CreateApplicationFromData(apps, loc, files, c)
// }

func writeFile(path string, contents []byte, c *check.C) {
	err := ioutil.WriteFile(path, contents, defaults.SharedReadWriteMask)
	c.Assert(err, check.IsNil)
}

func newHubApps(apps libapp.Applications) libapp.Applications {
	return legacyHubApps{Applications: apps}
}

func (r legacyHubApps) GetApp(loc loc.Locator) (*libapp.Application, error) {
	app, err := r.Applications.GetApp(loc)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	manifest := app.Manifest
	// Enterprise Hub runs an old version of gravity which strips down manifest details
	// it does not understand
	manifest.SystemOptions = nil
	app.Manifest = manifest
	return app, nil
}

// legacyHubApps implements the libapp.Applications interface but replaces
// the GetApp API to mimic the behavior of the legacy enterprise hub - namely,
// that it does not understand the recent versions of the manifest and strips
// away SystemOptions which is used to detect the planet package
type legacyHubApps struct {
	libapp.Applications
}

// func createPlanetDockerImage(dir, tag string, c *check.C) {
// 	dockerfileBytes := []byte(`
// FROM scratch
// COPY ./orbit.manifest.json /etc/planet/
// `)
// 	planetManifestBytes := []byte(`{}`)
//
// 	writeFile(filepath.Join(dir, "Dockerfile"), dockerfileBytes, c)
// 	writeFile(filepath.Join(dir, "orbit.manifest.json"), planetManifestBytes, c)
// 	buildDockerImage(dir, tag, c)
// }
//
// func removePlanetDockerImage(c *check.C) {
// 	removeDockerImage(planetTag)
// 	removeDockerImage(planetPlainTag)
// }
//
// func buildDockerImage(dir, tag string, c *check.C) {
// 	out, err := exec.Command("docker", "build", "-t", tag, dir).CombinedOutput()
// 	c.Assert(err, check.IsNil, check.Commentf(string(out)))
// }
//
// func removeDockerImage(tag string) {
// 	exec.Command("docker", "rmi", tag).Run()
// }

func unpackTarball(path, unpackedDir string, c *check.C) *localenv.LocalEnvironment {
	f, err := os.Open(path)
	c.Assert(err, check.IsNil)
	defer f.Close()
	err = dockerarchive.Untar(f, unpackedDir, archive.DefaultOptions())
	c.Assert(err, check.IsNil)
	env, err := localenv.New(unpackedDir)
	c.Assert(err, check.IsNil)
	return env
}

func checkDockerAvailable() bool {
	_, err := docker.NewClientFromEnv()
	return err == nil
}

const (
	manifestWithBase = `apiVersion: cluster.gravitational.io/v2
kind: Cluster
baseImage: gravity:5.5.0
metadata:
  name: test
  resourceVersion: 1.0.0`

	manifestWithoutBase = `apiVersion: cluster.gravitational.io/v2
kind: Cluster
metadata:
  name: test
  resourceVersion: 1.0.0`

	manifestInvalidBase = `apiVersion: cluster.gravitational.io/v2
kind: Cluster
baseImage: example:1.2.3
metadata:
  name: test
  resourceVersion: 1.0.0`

	planetTag      = "quay.io/gravitational/planet:0.0.2"
	planetPlainTag = "gravitational/planet:0.0.2"
)
