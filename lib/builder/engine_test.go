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
	"strings"
	"testing"
	"time"

	libapp "github.com/gravitational/gravity/lib/app"
	appservice "github.com/gravitational/gravity/lib/app/service"
	apptest "github.com/gravitational/gravity/lib/app/service/test"
	"github.com/gravitational/gravity/lib/archive"
	"github.com/gravitational/gravity/lib/defaults"
	"github.com/gravitational/gravity/lib/docker"
	"github.com/gravitational/gravity/lib/loc"
	"github.com/gravitational/gravity/lib/localenv"
	packtest "github.com/gravitational/gravity/lib/pack/test"
	"github.com/gravitational/gravity/lib/schema"
	"github.com/gravitational/gravity/lib/utils"

	"github.com/coreos/go-semver/semver"
	dockerarchive "github.com/docker/docker/pkg/archive"
	dockerapi "github.com/fsouza/go-dockerclient"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/gravitational/trace"
	"github.com/gravitational/version"
	check "gopkg.in/check.v1"
)

func TestBuilder(t *testing.T) { check.TestingT(t) }

type BuilderSuite struct{}

var _ = check.Suite(&BuilderSuite{})

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
			Logger:   utils.NewTestLogger(c),
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

type InstallerBuilderSuite struct {
	builderCommon
}

var _ = check.Suite(&InstallerBuilderSuite{})

func (s *InstallerBuilderSuite) SetUpTest(c *check.C) {
	s.builderCommon.init(c)
}

func (s *InstallerBuilderSuite) TearDownTest(c *check.C) {
	s.builderCommon.close()
}

func (s *InstallerBuilderSuite) TestBuildInstallerWithDefaultPlanetPackage(c *check.C) {
	// setup
	appLoc, depAppLoc := newLoc("app:0.0.1"), newLoc("app-dep:0.0.1")
	depApp := apptest.SystemApplication(depAppLoc).Build()
	app := apptest.DefaultClusterApplication(appLoc).
		WithSchemaPackageDependencies(newLoc("gravity:0.0.1")).
		WithAppDependencies(depApp).
		Build()
	apptest.CreateApplicationDependencies(apptest.AppRequest{
		App:      app,
		Apps:     s.remoteEnv.Apps,
		Packages: s.remoteEnv.Packages,
	}, c)
	writeManifestFile(app.Manifest, s.appDir, c)

	// verify
	b := s.newClusterBuilder(c)
	s.build(c, b)
}

func (s *InstallerBuilderSuite) TestBuildInstallerWithMixedVersionPackages(c *check.C) {
	// setup
	// Simulate a workflow with packages/applications of mixed versions
	version.Init("0.0.1")
	runtimeApp1 := apptest.RuntimeApplication(
		newLoc("kubernetes:0.0.1"),
		newLoc("planet:0.0.1")).Build()
	// and at least one version available above the requested runtime version
	runtimeApp2 := apptest.RuntimeApplication(
		newLoc("kubernetes:0.0.2"),
		newLoc("planet:0.0.2")).Build()
	app := apptest.ClusterApplicationWithRuntimePlaceholder(newLoc("app:0.0.1")).
		WithSchemaPackageDependencies(newLoc("gravity:0.0.1")).
		Build()
	apptest.CreateApplicationDependencies(apptest.AppRequest{
		App:      app,
		Apps:     s.buildEnv.Apps,
		Packages: s.buildEnv.Packages,
	}, c)
	writeManifestFile(app.Manifest, s.appDir, c)
	for _, app := range []apptest.App{runtimeApp1, runtimeApp2} {
		apptest.CreateApplication(apptest.AppRequest{
			App:      app,
			Apps:     s.buildEnv.Apps,
			Packages: s.buildEnv.Packages,
		}, c)
	}

	// verify
	outputPath := filepath.Join(s.buildEnv.StateDir, "app.tar")
	b := s.newClusterBuilder(c)
	s.build(c, b)

	unpackDir := c.MkDir()
	tarballEnv := unpackTarball(outputPath, unpackDir, c)
	defer tarballEnv.Close()

	packtest.VerifyPackages(tarballEnv.Packages, newLocs(
		"planet:0.0.1",
		"app:0.0.1",
		"gravity:0.0.1",
		"kubernetes:0.0.1",
	), c)
}

func (s *InstallerBuilderSuite) TestBuildInstallerWithPackagesInCache(c *check.C) {
	// setup
	// Simulate a development workflow with packages/applications
	// explicitly cached (but also unavailable in the remote hub)
	runtimeApp := apptest.RuntimeApplication(newLoc("kubernetes:0.0.2-dev.1"), newLoc("planet:0.0.1")).Build()
	app := apptest.ClusterApplication(newLoc("app:0.0.1"), runtimeApp).
		WithSchemaPackageDependencies(newLoc("gravity:0.0.1")).
		Build()
	apptest.CreateApplicationDependencies(apptest.AppRequest{
		App:      app,
		Apps:     s.buildEnv.Apps,
		Packages: s.buildEnv.Packages,
	}, c)
	writeManifestFile(app.Manifest, s.appDir, c)

	// verify
	b := s.newClusterBuilder(c)
	s.build(c, b)
}

func (s *InstallerBuilderSuite) TestBuildInstallerWithIntermediateHops(c *check.C) {
	// setup
	runtimeApp1 := apptest.RuntimeApplication(
		newLoc("kubernetes:0.0.1"),
		newLoc("planet:0.0.1")).
		WithSchemaPackageDependencies(newLoc("gravity:0.0.1")).
		Build()
	// and at least one version available above the requested runtime version
	runtimeApp2 := apptest.RuntimeApplication(
		newLoc("kubernetes:0.0.2"),
		newLoc("planet:0.0.2")).
		WithSchemaPackageDependencies(newLoc("gravity:0.0.2")).
		Build()
	app := apptest.ClusterApplication(newLoc("app:0.0.1"), runtimeApp2).
		Build()
	apptest.CreateApplicationDependencies(apptest.AppRequest{
		App:      app,
		Apps:     s.remoteEnv.Apps,
		Packages: s.remoteEnv.Packages,
	}, c)
	writeManifestFile(app.Manifest, s.appDir, c)
	apptest.CreateApplication(apptest.AppRequest{
		App:      runtimeApp1,
		Apps:     s.remoteEnv.Apps,
		Packages: s.remoteEnv.Packages,
	}, c)

	// verify
	b := s.newClusterBuilder(c, func(c *Config) {
		c.UpgradeVia = []string{"0.0.1"}
	})
	outputPath := filepath.Join(s.buildEnv.StateDir, "app.tar")
	err := b.Build(context.TODO(), ClusterRequest{
		OutputPath: outputPath,
		SourcePath: s.appDir,
	})
	c.Assert(err, check.IsNil)

	unpackDir := c.MkDir()
	tarballEnv := unpackTarball(outputPath, unpackDir, c)
	defer tarballEnv.Close()

	packtest.VerifyPackages(tarballEnv.Packages, newLocs(
		"planet:0.0.2",
		"planet:0.0.1",
		"app:0.0.1",
		"gravity:0.0.1",
		"gravity:0.0.2",
		"kubernetes:0.0.1",
		"kubernetes:0.0.2",
	), c)
	verifyIntermediateVersionsInManifest(unpackDir, []schema.IntermediateVersion{
		{
			Version: mustVer("0.0.1"),
			Dependencies: schema.Dependencies{
				Packages: []schema.Dependency{
					{Locator: newLoc("planet:0.0.1")},
					{Locator: newLoc("gravity:0.0.1")},
				},
			},
		},
	}, c)
}

func (s *InstallerBuilderSuite) TestBuildInstallerWithDefaultPlanetPackageFromLegacyHub(c *check.C) {
	// setup
	runtimeApp := apptest.RuntimeApplication(newLoc("kubernetes:0.0.1"), newLoc("planet:0.0.1")).Build()
	app := apptest.ClusterApplication(newLoc("app:0.0.1"), runtimeApp).
		WithSchemaPackageDependencies(newLoc("gravity:0.0.1")).
		Build()
	apptest.CreateApplicationDependencies(apptest.AppRequest{
		App:      app,
		Apps:     s.remoteEnv.Apps,
		Packages: s.remoteEnv.Packages,
	}, c)
	writeManifestFile(app.Manifest, s.appDir, c)

	// verify
	b := s.newClusterBuilder(c, func(c *Config) {
		c.Syncer = NewPackSyncer(s.remoteEnv.Packages, newHubApps(s.remoteEnv.Apps), repository)
	})
	s.build(c, b)
}

type CustomImageBuilderSuite struct {
	builderCommon
	image  docker.TestImage
	client *dockerapi.Client
}

var _ = check.Suite(&CustomImageBuilderSuite{})

func (s *CustomImageBuilderSuite) SetUpSuite(c *check.C) {
	s.client = docker.TestRequiresDocker(c)
}

func (s *CustomImageBuilderSuite) SetUpTest(c *check.C) {
	const dockerFile = `FROM scratch
COPY ./orbit.manifest.json /etc/planet/
`
	s.image = docker.TestImage{
		DockerImage: planetImageRef,
		Items: []*archive.Item{
			archive.ItemFromStringMode("orbit.manifest.json", "{}", defaults.SharedReadWriteMask),
			archive.ItemFromStringMode("Dockerfile", dockerFile, defaults.SharedReadWriteMask),
		},
	}
	s.builderCommon.init(c)
	docker.GenerateTestDockerImageFromSpec(s.client, s.image, c)
}

func (s *CustomImageBuilderSuite) TearDownTest(*check.C) {
	s.builderCommon.close()
	_ = s.client.RemoveImage(planetImageRef.String())
	_ = s.client.RemoveImage(planetAutogeneratedImageRef)
}

func (s *CustomImageBuilderSuite) TestBuildInstallerWithCustomGlobalPlanetPackage(c *check.C) {
	// setup
	runtimeApp := apptest.RuntimeApplication(newLoc("kubernetes:0.0.1"), newLoc("planet:0.0.1")).Build()
	appLoc := newLoc("app:0.0.1")
	app := apptest.ClusterApplication(appLoc, runtimeApp).
		WithSchemaPackageDependencies(newLoc("gravity:0.0.1")).
		Build()
	app.Manifest.SystemOptions.BaseImage = planetImageRef.String()
	apptest.CreateApplicationDependencies(apptest.AppRequest{
		App:      app,
		Apps:     s.remoteEnv.Apps,
		Packages: s.remoteEnv.Packages,
	}, c)
	writeManifestFile(app.Manifest, s.appDir, c)

	// verify
	b := s.newClusterBuilder(c, func(c *Config) {
		c.Syncer = NewPackSyncer(s.remoteEnv.Packages, newHubApps(s.remoteEnv.Apps), repository)
	})
	outputPath := filepath.Join(s.buildEnv.StateDir, "app.tar")
	err := b.Build(context.TODO(), ClusterRequest{
		OutputPath: outputPath,
		SourcePath: s.appDir,
		Vendor: appservice.VendorRequest{
			PackageName:      appLoc.Name,
			PackageVersion:   appLoc.Version,
			VendorRuntime:    true,
			ManifestPath:     filepath.Join(s.appDir, defaults.ManifestFileName),
			ResourcePatterns: []string{defaults.VendorPattern},
		},
	})
	c.Assert(err, check.IsNil)

	unpackDir := c.MkDir()
	tarballEnv := unpackTarball(outputPath, unpackDir, c)
	defer tarballEnv.Close()

	packtest.VerifyPackages(tarballEnv.Packages, newLocs(
		"planet:0.0.2",
		"app:0.0.1",
		"gravity:0.0.1",
		"kubernetes:0.0.1",
	), c)
}

func (s *CustomImageBuilderSuite) TestBuildInstallerWithCustomPerNodePlanetPackage(c *check.C) {
	// setup
	runtimeApp := apptest.RuntimeApplication(newLoc("kubernetes:0.0.1"), newLoc("planet:0.0.1")).Build()
	appLoc := newLoc("app:0.0.1")
	app := apptest.ClusterApplication(appLoc, runtimeApp).
		WithSchemaPackageDependencies(newLoc("gravity:0.0.1")).
		Build()
	// Use a custom runtime container for the node
	app.Manifest.NodeProfiles[0].SystemOptions = &schema.SystemOptions{
		BaseImage: planetImageRef.String(),
	}
	apptest.CreateApplicationDependencies(apptest.AppRequest{
		App:      app,
		Apps:     s.remoteEnv.Apps,
		Packages: s.remoteEnv.Packages,
	}, c)
	writeManifestFile(app.Manifest, s.appDir, c)

	// verify
	outputPath := filepath.Join(s.buildEnv.StateDir, "app.tar")
	b := s.newClusterBuilder(c, func(c *Config) {
		c.Syncer = NewPackSyncer(s.remoteEnv.Packages, newHubApps(s.remoteEnv.Apps), repository)
	})
	err := b.Build(context.TODO(), ClusterRequest{
		OutputPath: outputPath,
		SourcePath: s.appDir,
		Vendor: appservice.VendorRequest{
			PackageName:      appLoc.Name,
			PackageVersion:   appLoc.Version,
			VendorRuntime:    true,
			ManifestPath:     filepath.Join(s.appDir, defaults.ManifestFileName),
			ResourcePatterns: []string{defaults.VendorPattern},
		},
	})
	c.Assert(err, check.IsNil)

	unpackDir := c.MkDir()
	tarballEnv := unpackTarball(outputPath, unpackDir, c)
	defer tarballEnv.Close()

	packtest.VerifyPackages(tarballEnv.Packages, newLocs(
		// Per-node custom planet configuration adds to the list of planet packages
		"quay.io-gravitational-planet:0.0.2",
		"planet:0.0.1",
		"app:0.0.1",
		"gravity:0.0.1",
		"kubernetes:0.0.1",
	), c)
}

type builderCommon struct {
	buildEnv, remoteEnv *localenv.LocalEnvironment
	appDir              string
}

func (s *builderCommon) newClusterBuilder(c *check.C, opts ...func(*Config)) *ClusterBuilder {
	config := Config{
		Logger:           utils.NewTestLogger(c),
		Env:              s.buildEnv,
		SkipVersionCheck: true,
		Repository:       repository,
		Syncer:           NewPackSyncer(s.remoteEnv.Packages, s.remoteEnv.Apps, repository),
	}
	for _, opt := range opts {
		opt(&config)
	}
	b, err := NewClusterBuilder(config)
	c.Assert(err, check.IsNil)
	return b
}

func (s *builderCommon) init(c *check.C) {
	s.remoteEnv = newEnviron(c)
	s.buildEnv = newEnviron(c)
	s.appDir = c.MkDir()
}

func (s *builderCommon) close() {
	s.remoteEnv.Close()
	s.buildEnv.Close()
}

func (s *builderCommon) build(c *check.C, b *ClusterBuilder) {
	err := b.Build(context.TODO(), ClusterRequest{
		OutputPath: filepath.Join(s.buildEnv.StateDir, "app.tar"),
		SourcePath: s.appDir,
	})
	c.Assert(err, check.IsNil)
}

func newLocs(locs ...string) (result []loc.Locator) {
	for _, l := range locs {
		result = append(result, newLoc(l))
	}
	return result
}

func newLoc(nameVersion string) loc.Locator {
	parts := strings.Split(nameVersion, ":")
	return loc.Locator{
		Repository: defaults.SystemAccountOrg,
		Name:       parts[0],
		Version:    parts[1],
	}
}

func mustVer(v string) semver.Version {
	return *semver.New(v)
}

func newEnviron(c *check.C) *localenv.LocalEnvironment {
	env, err := localenv.New(c.MkDir())
	c.Assert(err, check.IsNil)
	c.Assert(env.Packages.UpsertRepository(defaults.SystemAccountOrg, time.Time{}), check.IsNil)
	return env
}

func writeManifestFile(m schema.Manifest, path string, c *check.C) {
	bytes, err := json.MarshalIndent(m, "  ", " ")
	c.Assert(err, check.IsNil)
	c.Assert(ioutil.WriteFile(
		filepath.Join(path, defaults.ManifestFileName),
		bytes,
		defaults.SharedReadWriteMask,
	), check.IsNil)
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

func verifyIntermediateVersionsInManifest(dir string, expectedVersions []schema.IntermediateVersion, c *check.C) {
	bytes, err := ioutil.ReadFile(filepath.Join(dir, defaults.ManifestFileName))
	c.Assert(err, check.IsNil)
	m, err := schema.ParseManifestYAMLNoValidate(bytes)
	c.Assert(err, check.IsNil)
	opts := []cmp.Option{
		cmpopts.SortSlices(func(a, b schema.Dependency) bool {
			return a.Locator.Name < b.Locator.Name
		}),
	}
	if !cmp.Equal(m.SystemOptions.Dependencies.IntermediateVersions, expectedVersions, opts...) {
		c.Error("Versions differ:", cmp.Diff(m.SystemOptions.Dependencies.IntermediateVersions, expectedVersions))
	}
}

var planetImageRef = loc.DockerImage{
	Repository: "quay.io/gravitational/planet",
	Tag:        "0.0.2",
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

	// Auto-generated planet docker image reference for the image reference above
	planetAutogeneratedImageRef = "gravitational/planet:0.0.2"
	repository                  = "repository"
)
