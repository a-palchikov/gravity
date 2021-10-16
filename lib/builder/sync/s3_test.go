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

package sync

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"

	apptest "github.com/gravitational/gravity/lib/app/service/test"
	"github.com/gravitational/gravity/lib/archive"
	"github.com/gravitational/gravity/lib/builder"
	"github.com/gravitational/gravity/lib/builder/suite"
	"github.com/gravitational/gravity/lib/loc"
	"github.com/gravitational/trace"

	"gopkg.in/check.v1"
)

type InstallerBuilderSuiteS3 struct {
	buildEnv  *localEnviron
	remoteEnv *testHub
	s         *suite.InstallerBuilderSuite
}

var _ = check.Suite(&InstallerBuilderSuiteS3{})

func (s *InstallerBuilderSuiteS3) SetUpTest(c *check.C) {
	s.buildEnv = newEnviron(c)
	s.remoteEnv = newTestHub()
	s.s = &suite.InstallerBuilderSuite{
		BuilderCommon: suite.BuilderCommon{
			BuildEnv:  s.buildEnv,
			RemoteEnv: s.remoteEnv,
			B:         s.newClusterBuilder,
		},
	}
	s.s.SetUpTest(c)
}

func (s *InstallerBuilderSuiteS3) TearDownTest(c *check.C) {
	s.s.TearDownTest(c)
}

func (s *InstallerBuilderSuiteS3) TestBuildInstallerWithDefaultPlanetPackageS3(c *check.C) {
	s.s.BuildInstallerWithDefaultPlanetPackage(c)
}

func (s *InstallerBuilderSuiteS3) TestBuildInstallerWithMixedVersionPackages(c *check.C) {
	s.s.BuildInstallerWithMixedVersionPackages(c)
}

func (s *InstallerBuilderSuiteS3) TestBuildInstallerWithPackagesInCache(c *check.C) {
	s.s.BuildInstallerWithPackagesInCache(c)
}

func (s *InstallerBuilderSuiteS3) TestBuildInstallerWithIntermediateHopsS3(c *check.C) {
	s.s.BuildInstallerWithIntermediateHops(c)
}

func (s *InstallerBuilderSuiteS3) newClusterBuilder(c *check.C, opts ...func(*builder.Config)) *builder.ClusterBuilder {
	syncer := NewS3(s.remoteEnv)
	return newClusterBuilder(c, s.buildEnv, syncer, opts...)
}

type CustomImageBuilderSuiteS3 struct {
	buildEnv  *localEnviron
	remoteEnv *testHub
	s         *suite.CustomImageBuilderSuite
}

var _ = check.Suite(&CustomImageBuilderSuiteS3{})

func (s *CustomImageBuilderSuiteS3) SetUpSuite(c *check.C) {
	s.s = &suite.CustomImageBuilderSuite{
		BuilderCommon: suite.BuilderCommon{
			B: s.newClusterBuilder,
		},
	}
	s.s.SetUpSuite(c)
}

func (s *CustomImageBuilderSuiteS3) SetUpTest(c *check.C) {
	s.buildEnv = newEnviron(c)
	s.remoteEnv = newTestHub()
	s.s.BuilderCommon.BuildEnv = s.buildEnv
	s.s.BuilderCommon.RemoteEnv = s.remoteEnv
	s.s.SetUpTest(c)
}

func (s *CustomImageBuilderSuiteS3) TearDownTest(c *check.C) {
	s.s.TearDownTest(c)
}

func (s *CustomImageBuilderSuiteS3) TestBuildInstallerWithCustomGlobalPlanetPackage(c *check.C) {
	s.s.BuildInstallerWithCustomGlobalPlanetPackage(c)
}

func (s *CustomImageBuilderSuiteS3) TestBuildInstallerWithCustomPerNodePlanetPackage(c *check.C) {
	s.s.BuildInstallerWithCustomPerNodePlanetPackage(c)
}

func (s *CustomImageBuilderSuiteS3) newClusterBuilder(c *check.C, opts ...func(*builder.Config)) *builder.ClusterBuilder {
	syncer := NewS3(s.remoteEnv)
	return newClusterBuilder(c, s.buildEnv, syncer, opts...)
}

// Get returns the underlying package store as a tarball.
// Implements HubGetter
func (r *testHub) Get(pkg loc.Locator) (rc io.ReadCloser, err error) {
	tarball, ok := r.runtimes[pkg]
	if !ok {
		return nil, trace.BadParameter("no telekube %v in store", pkg)
	}
	return ioutil.NopCloser(bytes.NewReader(tarball)), nil
}

func (r *testHub) CreateApp(c *check.C, app apptest.App, runtimes ...apptest.App) {
	telekubeAppLoc := newLoc(fmt.Sprint("telekube:", app.Base.Locator().Version))
	telekubeApp := apptest.ClusterApplication(telekubeAppLoc, *app.Base).
		WithDependencies(app.Dependencies).
		Build()
	r.storeTarball(c, telekubeApp)
	for _, runtime := range runtimes {
		telekubeAppLoc := newLoc(fmt.Sprint("telekube:", runtime.Locator().Version))
		telekubeApp := apptest.ClusterApplication(telekubeAppLoc, runtime).Build()
		r.storeTarball(c, telekubeApp)
	}
}

func newTestHub() *testHub {
	return &testHub{
		runtimes: make(map[loc.Locator][]byte),
	}
}

func (r *testHub) Close() error { return nil }

func (r *testHub) storeTarball(c *check.C, app apptest.App) {
	env := newEnviron(c)
	defer env.Close()

	apptest.CreateApplication(apptest.AppRequest{
		App:      app,
		Apps:     env.Apps,
		Packages: env.Packages,
	}, c)

	//packtest.DumpPackages(env.Packages, os.Stdout)

	var w bytes.Buffer
	c.Assert(archive.CompressDirectory(env.StateDir, &w), check.IsNil)
	r.runtimes[app.Locator()] = w.Bytes()
	fmt.Println("Done storing tarball.")
}

// testHub is a package store which also acts as a hub
type testHub struct {
	runtimes map[loc.Locator][]byte
}
