/*
Copyright 2021 Gravitational, Inc.

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

package suite

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	appservice "github.com/gravitational/gravity/lib/app/service"
	apptest "github.com/gravitational/gravity/lib/app/service/test"
	"github.com/gravitational/gravity/lib/archive"
	"github.com/gravitational/gravity/lib/builder"
	"github.com/gravitational/gravity/lib/defaults"
	"github.com/gravitational/gravity/lib/docker"
	"github.com/gravitational/gravity/lib/loc"
	loctest "github.com/gravitational/gravity/lib/loc/test"
	"github.com/gravitational/gravity/lib/localenv"
	localenvtest "github.com/gravitational/gravity/lib/localenv/test"
	packtest "github.com/gravitational/gravity/lib/pack/test"
	"github.com/gravitational/gravity/lib/schema"
	"github.com/gravitational/version"

	"github.com/coreos/go-semver/semver"
	dockerarchive "github.com/docker/docker/pkg/archive"
	dockerapi "github.com/fsouza/go-dockerclient"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gopkg.in/check.v1"
)

type InstallerBuilderSuite struct {
	BuilderCommon
}

func (s *InstallerBuilderSuite) SetUpTest(c *check.C) {
	s.AppDir = c.MkDir()
}

func (s *InstallerBuilderSuite) TearDownTest(c *check.C) {
	s.BuilderCommon.Close()
}

func (s *InstallerBuilderSuite) BuildInstallerWithDefaultPlanetPackage(c *check.C) {
	// setup
	appLoc, depAppLoc := newLoc("app:0.0.1"), newLoc("app-dep:0.0.1")
	depApp := apptest.SystemApplication(depAppLoc).Build()
	runtimeApp := apptest.RuntimeApplication(newLoc("kubernetes:1.0.0"), newLoc("planet:1.0.0")).
		WithSchemaPackageDependencies(newLoc("gravity:0.0.1")).
		Build()
	app := apptest.ClusterApplication(appLoc, runtimeApp).
		WithAppDependencies(depApp).
		Build()
	s.RemoteEnv.CreateApp(c, app)

	// verify
	WriteManifestFile(c, app.Manifest, s.AppDir)
	s.build(c, s.B(c))
}

func (s *InstallerBuilderSuite) BuildInstallerWithMixedVersionPackages(c *check.C) {
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
	s.BuildEnv.CreateApp(c, app, runtimeApp1, runtimeApp2)

	// verify
	WriteManifestFile(c, app.Manifest, s.AppDir)
	outputPath := s.build(c, s.B(c))

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

func (s *InstallerBuilderSuite) BuildInstallerWithPackagesInCache(c *check.C) {
	// setup
	// Simulate a development workflow with packages/applications
	// explicitly cached (but also unavailable in the remote hub)
	runtimeApp := apptest.RuntimeApplication(newLoc("kubernetes:0.0.2-dev.1"), newLoc("planet:0.0.1")).Build()
	app := apptest.ClusterApplication(newLoc("app:0.0.1"), runtimeApp).
		WithSchemaPackageDependencies(newLoc("gravity:0.0.1")).
		Build()
	s.BuildEnv.CreateApp(c, app)

	// verify
	WriteManifestFile(c, app.Manifest, s.AppDir)
	s.build(c, s.B(c))
}

func (s *InstallerBuilderSuite) BuildInstallerWithIntermediateHops(c *check.C) {
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
	s.RemoteEnv.CreateApp(c, app, runtimeApp1)

	// verify
	WriteManifestFile(c, app.Manifest, s.AppDir)
	b := s.B(c, func(c *builder.Config) {
		c.UpgradeVia = []string{"0.0.1"}
	})
	outputPath := s.build(c, b)

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

func (s *InstallerBuilderSuite) build(c *check.C, b *builder.ClusterBuilder) (outputPath string) {
	outputPath = filepath.Join(s.BuildEnv.Path(), "app.tar")
	err := b.Build(context.TODO(), builder.ClusterRequest{
		OutputPath: outputPath,
		SourcePath: s.AppDir,
	})
	c.Assert(err, check.IsNil)
	return outputPath
}

type CustomImageBuilderSuite struct {
	BuilderCommon

	image  docker.TestImage
	client *dockerapi.Client
}

func (s *CustomImageBuilderSuite) SetUpSuite(c *check.C) {
	s.client = docker.TestRequiresDocker(c)
}

func (s *CustomImageBuilderSuite) SetUpTest(c *check.C) {
	const dockerFile = `FROM scratch
COPY ./orbit.manifest.json /etc/planet/
`
	s.AppDir = c.MkDir()
	s.image = docker.TestImage{
		DockerImage: planetImageRef,
		Items: []*archive.Item{
			archive.ItemFromStringMode("orbit.manifest.json", "{}", defaults.SharedReadWriteMask),
			archive.ItemFromStringMode("Dockerfile", dockerFile, defaults.SharedReadWriteMask),
		},
	}
	docker.GenerateTestDockerImageFromSpec(s.client, s.image, c)
}

func (s *CustomImageBuilderSuite) TearDownTest(*check.C) {
	s.BuilderCommon.Close()
	_ = s.client.RemoveImage(planetImageRef.String())
	_ = s.client.RemoveImage(planetAutogeneratedImageRef)
}

func (s *CustomImageBuilderSuite) BuildInstallerWithCustomGlobalPlanetPackage(c *check.C) {
	// setup
	runtimeApp := apptest.RuntimeApplication(newLoc("kubernetes:0.0.1"), newLoc("planet:0.0.1")).Build()
	appLoc := newLoc("app:0.0.1")
	app := apptest.ClusterApplication(appLoc, runtimeApp).
		WithSchemaPackageDependencies(newLoc("gravity:0.0.1")).
		Build()
	app.Manifest.SystemOptions.BaseImage = planetImageRef.String()
	s.RemoteEnv.CreateApp(c, app)

	// verify
	WriteManifestFile(c, app.Manifest, s.AppDir)
	outputPath := s.buildWithRuntimeVendor(c, s.B(c), appLoc)
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

func (s *CustomImageBuilderSuite) BuildInstallerWithCustomPerNodePlanetPackage(c *check.C) {
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
	s.RemoteEnv.CreateApp(c, app)

	// verify
	WriteManifestFile(c, app.Manifest, s.AppDir)
	outputPath := s.buildWithRuntimeVendor(c, s.B(c), appLoc)
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

func (s *CustomImageBuilderSuite) build(c *check.C, b *builder.ClusterBuilder) (outputPath string) {
	outputPath = filepath.Join(s.BuildEnv.Path(), "app.tar")
	err := b.Build(context.TODO(), builder.ClusterRequest{
		OutputPath: outputPath,
		SourcePath: s.AppDir,
	})
	c.Assert(err, check.IsNil)
	return outputPath
}

func (s *CustomImageBuilderSuite) buildWithRuntimeVendor(c *check.C, b *builder.ClusterBuilder, appLoc loc.Locator) (outputPath string) {
	outputPath = filepath.Join(s.BuildEnv.Path(), "app.tar")
	err := b.Build(context.TODO(), builder.ClusterRequest{
		OutputPath: outputPath,
		SourcePath: s.AppDir,
		Vendor: appservice.VendorRequest{
			PackageName:      appLoc.Name,
			PackageVersion:   appLoc.Version,
			VendorRuntime:    true,
			ManifestPath:     filepath.Join(s.AppDir, defaults.ManifestFileName),
			ResourcePatterns: []string{defaults.VendorPattern},
		},
	})
	c.Assert(err, check.IsNil)
	return outputPath
}

// WriteManifestFile serializes the specified manifest m at path
// as JSON payload
func WriteManifestFile(c *check.C, m schema.Manifest, path string) {
	bytes, err := json.MarshalIndent(m, "  ", " ")
	c.Assert(err, check.IsNil)
	c.Assert(ioutil.WriteFile(
		filepath.Join(path, defaults.ManifestFileName),
		bytes,
		defaults.SharedReadWriteMask,
	), check.IsNil)
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

func mustVer(v string) semver.Version {
	return *semver.New(v)
}

const (
	// Auto-generated planet docker image reference for the image reference above
	planetAutogeneratedImageRef = "gravitational/planet:0.0.2"

	// Repository names the test repository
	Repository = "repository"
)

var (
	newLoc     = loctest.NewWithSystemRepository
	newLocs    = loctest.NewLocsWithSystemRepository
	newEnviron = localenvtest.New

	planetImageRef = loc.DockerImage{
		Repository: "quay.io/gravitational/planet",
		Tag:        "0.0.2",
	}
)

func (r BuilderCommon) Close() {
	r.BuildEnv.Close()
	r.RemoteEnv.Close()
}

type BuilderCommon struct {
	BuildEnv  localBuilderContext
	RemoteEnv builderContext
	B         func(c *check.C, opts ...func(*builder.Config)) *builder.ClusterBuilder
	AppDir    string
}

type localBuilderContext interface {
	builderContext

	Path() string
}

type builderContext interface {
	io.Closer

	CreateApp(c *check.C, app apptest.App, runtimeApps ...apptest.App)
}
