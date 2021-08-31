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
	"os/exec"
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
	"github.com/gravitational/trace"
	"github.com/gravitational/version"

	"github.com/coreos/go-semver/semver"
	"github.com/sirupsen/logrus"
	check "gopkg.in/check.v1"
)

func TestBuilder(t *testing.T) {
	check.TestingT(t)
}

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
			Log:      newLogger(c),
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
	docker.TestRequiresDocker(c)

	// setup
	remoteEnv := newEnviron(c)
	defer remoteEnv.Close()
	buildEnv := newEnviron(c)
	defer buildEnv.Close()
	appDir := c.MkDir()
	manifestPath := filepath.Join(appDir, defaults.ManifestFileName)
	app := createClusterApplication(remoteEnv, c)

	manifestBytes, err := json.Marshal(app.Manifest)
	c.Assert(err, check.IsNil)
	writeFile(manifestPath, manifestBytes, c)

	b, err := NewClusterBuilder(Config{
		Log:              newLogger(c),
		Progress:         utils.DiscardProgress,
		Env:              buildEnv,
		SkipVersionCheck: true,
		Repository:       "repository",
		Syncer:           NewPackSyncer(remoteEnv.Packages, remoteEnv.Apps, "repository"),
	})
	c.Assert(err, check.IsNil)

	// verify
	err = b.Build(context.TODO(), ClusterRequest{
		OutputPath: filepath.Join(buildEnv.StateDir, "app.tar"),
		SourcePath: filepath.Dir(manifestPath),
	})
	c.Assert(err, check.IsNil)
}

func (s *InstallerBuilderSuite) TestBuildInstallerWithMixedVersionPackages(c *check.C) {
	docker.TestRequiresDocker(c)

	// setup
	remoteEnv := newEnviron(c)
	defer remoteEnv.Close()
	buildEnv := newEnviron(c)
	defer buildEnv.Close()
	appDir := c.MkDir()
	manifestPath := filepath.Join(appDir, defaults.ManifestFileName)

	// Force builder to use specific runtime version
	version.Init("0.0.1")
	outputPath := filepath.Join(buildEnv.StateDir, "app.tar")
	b, err := NewClusterBuilder(Config{
		Log:              newLogger(c),
		Progress:         utils.DiscardProgress,
		Env:              buildEnv,
		SkipVersionCheck: true,
		Repository:       "repository",
		Syncer:           NewPackSyncer(remoteEnv.Packages, remoteEnv.Apps, "repository"),
	})
	c.Assert(err, check.IsNil)

	// Simulate a workflow with packages/applications of mixed versions
	// and at least one version available above the requested runtime version
	createRuntimeApplicationWithVersion(buildEnv, "0.0.1", c)
	createRuntimeApplicationWithVersion(buildEnv, "0.0.2", c)
	clusterApp := createClusterApplicationWithRuntimePlaceholder(buildEnv, c)
	writeManifestFile(clusterApp.Manifest, manifestPath, c)

	// verify
	err = b.Build(context.TODO(), ClusterRequest{
		OutputPath: outputPath,
		SourcePath: filepath.Dir(manifestPath),
	})
	c.Assert(err, check.IsNil)

	unpackDir := c.MkDir()
	tarballEnv := unpackTarball(outputPath, unpackDir, c)
	defer tarballEnv.Close()

	packtest.VerifyPackages(tarballEnv.Packages,
		mustLoc(
			"gravitational.io/planet:0.0.1",
			"gravitational.io/app:0.0.1",
			"gravitational.io/app-dependency:0.0.1",
			"gravitational.io/gravity:0.0.1",
			"gravitational.io/kubernetes:0.0.1",
		), c)
}

func (s *InstallerBuilderSuite) TestBuildInstallerWithPackagesInCache(c *check.C) {
	docker.TestRequiresDocker(c)

	// setup
	remoteEnv := newEnviron(c)
	defer remoteEnv.Close()
	buildEnv := newEnviron(c)
	defer buildEnv.Close()
	appDir := c.MkDir()
	manifestPath := filepath.Join(appDir, defaults.ManifestFileName)

	b, err := NewClusterBuilder(Config{
		Log:              newLogger(c),
		Progress:         utils.DiscardProgress,
		Env:              buildEnv,
		SkipVersionCheck: true,
		Repository:       "repository",
		Syncer:           NewPackSyncer(remoteEnv.Packages, remoteEnv.Apps, "repository"),
	})
	c.Assert(err, check.IsNil)

	// Simulate a development workflow with packages/applications
	// explicitly cached (but also unavailable in the remote hub)
	md := defaultApplicationMetadata.withRuntime("0.0.2-dev.1")
	clusterApp := createClusterApplicationFromMetadata(buildEnv, md, c)
	writeManifestFile(clusterApp.Manifest, manifestPath, c)

	// verify
	err = b.Build(context.TODO(), ClusterRequest{
		OutputPath: filepath.Join(buildEnv.StateDir, "app.tar"),
		SourcePath: filepath.Dir(manifestPath),
	})
	c.Assert(err, check.IsNil)
}

func (s *InstallerBuilderSuite) TestBuildInstallerWithIntermediateHops(c *check.C) {
	docker.TestRequiresDocker(c)

	// setup
	remoteEnv := newEnviron(c)
	defer remoteEnv.Close()
	buildEnv := newEnviron(c)
	defer buildEnv.Close()
	appDir := c.MkDir()
	manifestPath := filepath.Join(appDir, defaults.ManifestFileName)

	outputPath := filepath.Join(buildEnv.StateDir, "app.tar")
	b, err := NewClusterBuilder(Config{
		Log:              newLogger(c),
		Progress:         utils.DiscardProgress,
		Env:              buildEnv,
		SkipVersionCheck: true,
		Repository:       "repository",
		Syncer:           NewPackSyncer(remoteEnv.Packages, remoteEnv.Apps, "repository"),
		UpgradeVia:       []string{"0.0.1"},
	})
	c.Assert(err, check.IsNil)

	createRuntimeApplicationWithVersion(remoteEnv, "0.0.1", c)
	md := defaultApplicationMetadata.withRuntime("0.0.2")
	clusterApp := createClusterApplicationFromMetadata(remoteEnv, md, c)
	writeManifestFile(clusterApp.Manifest, manifestPath, c)

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
		packtest.NewPackage("gravitational.io/app-dependency:0.0.1"),
	}, c)
}

func (s *BuilderSuite) TestBuildInstallerWithDefaultPlanetPackageFromLegacyHub(c *check.C) {
	docker.TestRequiresDocker(c)

	// setup
	remoteEnv := newEnviron(c)
	defer remoteEnv.Close()
	buildEnv := newEnviron(c)
	defer buildEnv.Close()
	appDir := c.MkDir()
	manifestPath := filepath.Join(appDir, defaults.ManifestFileName)

	b, err := NewClusterBuilder(Config{
		Log:              newLogger(c),
		Progress:         utils.DiscardProgress,
		Env:              buildEnv,
		SkipVersionCheck: true,
		Repository:       "repository",
		Syncer:           NewPackSyncer(remoteEnv.Packages, newHubApps(remoteEnv.Apps), "repository"),
	})
	c.Assert(err, check.IsNil)

	clusterApp := createClusterApplication(remoteEnv, c)
	writeManifestFile(clusterApp.Manifest, manifestPath, c)

	// verify
	err = b.Build(context.TODO(), ClusterRequest{
		OutputPath: filepath.Join(buildEnv.StateDir, "app.tar"),
		SourcePath: filepath.Dir(manifestPath),
	})
	c.Assert(err, check.IsNil)
}

func (s *CustomImageBuilderSuite) SetUpTest(c *check.C) {
	docker.TestRequiresDocker(c)
	dockerDir := c.MkDir()
	// FIXME(dima): use dockertest builder API
	createPlanetDockerImage(dockerDir, planetTag, c)
}

func (s *CustomImageBuilderSuite) TearDownTest(c *check.C) {
	// FIXME(dima): use dockertest builder API
	removePlanetDockerImage(c)
}

func (s *CustomImageBuilderSuite) TestBuildInstallerWithCustomGlobalPlanetPackage(c *check.C) {
	remoteEnv := newEnviron(c)
	defer remoteEnv.Close()
	buildEnv := newEnviron(c)
	defer buildEnv.Close()
	appDir := c.MkDir()
	manifestPath := filepath.Join(appDir, defaults.ManifestFileName)
	outputPath := filepath.Join(buildEnv.StateDir, "app.tar")
	b, err := NewClusterBuilder(Config{
		Log:              newLogger(c),
		Progress:         utils.DiscardProgress,
		Env:              buildEnv,
		SkipVersionCheck: true,
		Repository:       "repository",
		Syncer:           NewPackSyncer(remoteEnv.Packages, newHubApps(remoteEnv.Apps), "repository"),
	})
	c.Assert(err, check.IsNil)

	md := defaultApplicationMetadata.withBaseImage("quay.io/gravitational/planet:0.0.2")
	clusterApp := createClusterApplicationFromMetadata(remoteEnv, md, c)
	writeManifestFile(clusterApp.Manifest, manifestPath, c)

	// verify
	err = b.Build(context.TODO(), ClusterRequest{
		OutputPath: outputPath,
		SourcePath: filepath.Dir(manifestPath),
		Vendor: app.VendorRequest{
			VendorRuntime:    true,
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
		"gravitational.io/kubernetes:0.0.1",
		"gravitational.io/app-dependency:0.0.1",
	), c)
}

func (s *CustomImageBuilderSuite) TestBuildInstallerWithCustomPerNodePlanetPackage(c *check.C) {
	remoteEnv := newEnviron(c)
	defer remoteEnv.Close()
	buildEnv := newEnviron(c)
	defer buildEnv.Close()
	appDir := c.MkDir()
	manifestPath := filepath.Join(appDir, defaults.ManifestFileName)
	outputPath := filepath.Join(buildEnv.StateDir, "app.tar")
	b, err := NewClusterBuilder(Config{
		Log:              newLogger(c),
		Progress:         utils.DiscardProgress,
		Env:              buildEnv,
		SkipVersionCheck: true,
		Repository:       "repository",
		Syncer:           NewPackSyncer(remoteEnv.Packages, newHubApps(remoteEnv.Apps), "repository"),
	})
	c.Assert(err, check.IsNil)

	clusterApp := createClusterApplication(remoteEnv, c)
	clusterApp.Manifest.NodeProfiles = schema.NodeProfiles{
		{
			Name: "master",
			Labels: map[string]string{
				"node-role.kubernetes.io/master": "true",
			},
		},
		{
			Name: "node",
			SystemOptions: &schema.SystemOptions{
				BaseImage: "quay.io/gravitational/planet:0.0.2",
			},
		},
	}
	writeManifestFile(clusterApp.Manifest, manifestPath, c)

	// verify
	err = b.Build(context.TODO(), ClusterRequest{
		OutputPath: outputPath,
		SourcePath: manifestPath,
		Vendor: app.VendorRequest{
			VendorRuntime:    true,
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
		"gravitational.io/app-dependency:0.0.1",
		"gravitational.io/kubernetes:0.0.1",
	), c)
}

func newLogger(c *check.C) logrus.FieldLogger {
	return logrus.WithField("test", c.TestName())
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

func createRuntimeApplication(env *localenv.LocalEnvironment, c *check.C) *libapp.Application {
	return createRuntimeApplicationWithVersion(env, "0.0.1", c)
}

func createRuntimeApplicationWithVersion(env *localenv.LocalEnvironment, version string, c *check.C) *libapp.Application {
	return apptest.CreateApplication(apptest.AppRequest{
		App:      defaultApplicationMetadata.withRuntime(version).runtimeApp,
		Apps:     env.Apps,
		Packages: env.Packages,
	}, c)
}

var defaultApplicationMetadata = applicationMetadata{
	runtimeApp: apptest.RuntimeApplication(apptest.RuntimeApplicationLoc, apptest.RuntimePackageLoc).
		WithSchemaPackageDependencies(
			loc.Gravity.WithLiteralVersion(apptest.RuntimePackageLoc.Version),
			// Since we're replacing the package dependencies, we need to
			// keep the runtime package in the list
			apptest.RuntimePackageLoc,
		).
		Build(),
	appDepLoc: loc.MustParseLocator("gravitational.io/app-dependency:0.0.1"),
	appLoc:    loc.MustParseLocator("gravitational.io/app:0.0.1"),
}

func (r applicationMetadata) withBaseImage(ref string) applicationMetadata {
	r.baseImage = ref
	return r
}

func (r applicationMetadata) withRuntime(version string) applicationMetadata {
	r.runtimeApp = apptest.RuntimeApplication(
		apptest.RuntimeApplicationLoc.WithLiteralVersion(version),
		apptest.RuntimePackageLoc.WithLiteralVersion(version)).
		WithSchemaPackageDependencies(
			loc.Gravity.WithLiteralVersion(version),
			apptest.RuntimePackageLoc.WithLiteralVersion(version),
		).
		Build()
	return r
}

type applicationMetadata struct {
	runtimeApp apptest.App
	appDepLoc  loc.Locator
	appLoc     loc.Locator
	baseImage  string
}

func createClusterApplication(env *localenv.LocalEnvironment, c *check.C) apptest.App {
	return createClusterApplicationFromMetadata(env, defaultApplicationMetadata, c)
}

func createClusterApplicationFromMetadata(env *localenv.LocalEnvironment, md applicationMetadata, c *check.C) apptest.App {
	appDep := apptest.SystemApplication(md.appDepLoc).Build()
	clusterApp := apptest.ClusterApplication(md.appLoc, md.runtimeApp).
		WithDependencies(apptest.Dependencies{Apps: []apptest.App{appDep}}).
		Build()
	clusterApp.Manifest.SystemOptions.BaseImage = md.baseImage
	apptest.CreateApplicationDependencies(apptest.AppRequest{
		App:      clusterApp,
		Apps:     env.Apps,
		Packages: env.Packages,
	}, c)
	return clusterApp
}

func createClusterApplicationWithRuntimePlaceholder(env *localenv.LocalEnvironment, c *check.C) apptest.App {
	appDep := apptest.SystemApplication(defaultApplicationMetadata.appDepLoc).Build()
	apptest.CreateApplication(apptest.AppRequest{
		App:      appDep,
		Apps:     env.Apps,
		Packages: env.Packages,
	}, c)
	return apptest.ClusterApplicationWithRuntimePlaceholder(defaultApplicationMetadata.appLoc).
		WithDependencies(apptest.Dependencies{Apps: []apptest.App{appDep}}).
		Build()
}

func writeManifestFile(manifest schema.Manifest, path string, c *check.C) {
	bytes, err := json.Marshal(manifest)
	c.Assert(err, check.IsNil)
	err = ioutil.WriteFile(path, bytes, defaults.SharedReadWriteMask)
	c.Assert(err, check.IsNil)
}

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

// TODO(dima): swap to docker builder APIs
func createPlanetDockerImage(dir, tag string, c *check.C) {
	dockerfileBytes := []byte(`
FROM scratch
COPY ./orbit.manifest.json /etc/planet/
`)
	planetManifestBytes := []byte(`{}`)

	writeFile(filepath.Join(dir, "Dockerfile"), dockerfileBytes, c)
	writeFile(filepath.Join(dir, "orbit.manifest.json"), planetManifestBytes, c)
	buildDockerImage(dir, tag, c)
}

func removePlanetDockerImage(c *check.C) {
	removeDockerImage(planetTag)
	removeDockerImage(planetPlainTag)
}

func buildDockerImage(dir, tag string, c *check.C) {
	out, err := exec.Command("docker", "build", "-t", tag, dir).CombinedOutput()
	c.Assert(err, check.IsNil, check.Commentf(string(out)))
}

func removeDockerImage(tag string) {
	exec.Command("docker", "rmi", tag).Run()
}

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
