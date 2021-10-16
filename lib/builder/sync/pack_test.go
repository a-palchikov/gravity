package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	libapp "github.com/gravitational/gravity/lib/app"
	apptest "github.com/gravitational/gravity/lib/app/service/test"
	"github.com/gravitational/gravity/lib/builder"
	"github.com/gravitational/gravity/lib/builder/suite"
	"github.com/gravitational/gravity/lib/loc"
	"github.com/gravitational/gravity/lib/schema"
	"github.com/gravitational/trace"

	"gopkg.in/check.v1"
)

type InstallerBuilderSuitePack struct {
	buildEnv, remoteEnv *localEnviron
	s                   *suite.InstallerBuilderSuite
}

var _ = check.Suite(&InstallerBuilderSuitePack{})

func (s *InstallerBuilderSuitePack) SetUpTest(c *check.C) {
	s.buildEnv, s.remoteEnv = newEnviron(c), newEnviron(c)
	s.s = &suite.InstallerBuilderSuite{
		BuilderCommon: suite.BuilderCommon{
			BuildEnv:  s.buildEnv,
			RemoteEnv: s.remoteEnv,
			B:         s.newClusterBuilder,
		},
	}
	s.s.SetUpTest(c)
}

func (s *InstallerBuilderSuitePack) TearDownTest(c *check.C) {
	s.s.TearDownTest(c)
}

func (s *InstallerBuilderSuitePack) TestBuildInstallerWithDefaultPlanetPackagePack(c *check.C) {
	s.s.BuildInstallerWithDefaultPlanetPackage(c)
}

func (s *InstallerBuilderSuitePack) TestBuildInstallerWithMixedVersionPackages(c *check.C) {
	s.s.BuildInstallerWithMixedVersionPackages(c)
}

func (s *InstallerBuilderSuitePack) TestBuildInstallerWithPackagesInCache(c *check.C) {
	s.s.BuildInstallerWithPackagesInCache(c)
}

func (s *InstallerBuilderSuitePack) TestBuildInstallerWithIntermediateHops(c *check.C) {
	s.s.BuildInstallerWithIntermediateHops(c)
}

func (s *InstallerBuilderSuitePack) newClusterBuilder(c *check.C, opts ...func(*builder.Config)) *builder.ClusterBuilder {
	syncer := NewPack(s.remoteEnv.Packages, s.remoteEnv.Apps, suite.Repository)
	return newClusterBuilder(c, s.buildEnv, syncer, opts...)
}

func (s *InstallerBuilderSuitePack) TestBuildInstallerWithDefaultPlanetPackageFromLegacyHub(c *check.C) {
	// setup
	runtimeApp := apptest.RuntimeApplication(newLoc("kubernetes:0.0.1"), newLoc("planet:0.0.1")).Build()
	writeManifestFile(c, runtimeApp.Manifest)
	app := apptest.ClusterApplication(newLoc("app:0.0.1"), runtimeApp).
		WithSchemaPackageDependencies(newLoc("gravity:0.0.1")).
		Build()
	apptest.CreateApplicationDependencies(apptest.AppRequest{
		App:      app,
		Apps:     s.remoteEnv.Apps,
		Packages: s.remoteEnv.Packages,
	}, c)
	appDir := c.MkDir()
	suite.WriteManifestFile(c, app.Manifest, appDir)

	// verify
	b := s.newClusterBuilder(c, func(c *builder.Config) {
		c.Syncer = NewPack(s.remoteEnv.Packages, newHubApps(s.remoteEnv.Apps), suite.Repository)
	})
	err := b.Build(context.TODO(), builder.ClusterRequest{
		OutputPath: filepath.Join(s.buildEnv.StateDir, "app.tar"),
		SourcePath: appDir,
	})
	c.Assert(err, check.IsNil)
}

func writeManifestFile(c *check.C, m schema.Manifest) {
	bytes, err := json.MarshalIndent(m, "  ", " ")
	c.Assert(err, check.IsNil)
	fmt.Println("Manifest:\n", string(bytes))
}

type CustomImageBuilderSuitePack struct {
	buildEnv, remoteEnv *localEnviron
	s                   *suite.CustomImageBuilderSuite
}

var _ = check.Suite(&CustomImageBuilderSuitePack{})

func (s *CustomImageBuilderSuitePack) SetUpSuite(c *check.C) {
	s.s = &suite.CustomImageBuilderSuite{
		BuilderCommon: suite.BuilderCommon{
			B: s.newClusterBuilder,
		},
	}
	s.s.SetUpSuite(c)
}

func (s *CustomImageBuilderSuitePack) SetUpTest(c *check.C) {
	s.buildEnv, s.remoteEnv = newEnviron(c), newEnviron(c)
	s.s.BuilderCommon.BuildEnv = s.buildEnv
	s.s.BuilderCommon.RemoteEnv = s.remoteEnv
	s.s.SetUpTest(c)
}

func (s *CustomImageBuilderSuitePack) TearDownTest(c *check.C) {
	s.s.TearDownTest(c)
}

func (s *CustomImageBuilderSuitePack) TestBuildInstallerWithCustomGlobalPlanetPackage(c *check.C) {
	s.s.BuildInstallerWithCustomGlobalPlanetPackage(c)
}

func (s *CustomImageBuilderSuitePack) TestBuildInstallerWithCustomPerNodePlanetPackage(c *check.C) {
	s.s.BuildInstallerWithCustomPerNodePlanetPackage(c)
}

func (s *CustomImageBuilderSuitePack) newClusterBuilder(c *check.C, opts ...func(*builder.Config)) *builder.ClusterBuilder {
	syncer := NewPack(s.remoteEnv.Packages, s.remoteEnv.Apps, suite.Repository)
	return newClusterBuilder(c, s.buildEnv, syncer, opts...)
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
