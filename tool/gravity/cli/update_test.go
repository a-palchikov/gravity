/*
Copyright 2020 Gravitational, Inc.

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

package cli

import (
	appservice "github.com/gravitational/gravity/lib/app/service"
	apptest "github.com/gravitational/gravity/lib/app/service/test"
	"github.com/gravitational/gravity/lib/defaults"
	"github.com/gravitational/gravity/lib/loc"
	"github.com/gravitational/gravity/lib/localenv"
	"github.com/gravitational/gravity/lib/ops/opsservice"

	"gopkg.in/check.v1"
)

type S struct{}

var _ = check.Suite(&S{})

func (s *S) TestGetsUpdateLatestPackage(c *check.C) {
	var args localenv.TarballEnvironmentArgs
	var emptyPackagePattern string

	// exercise
	loc, err := getUpdatePackage(args, emptyPackagePattern, clusterApp)
	c.Assert(err, check.IsNil)

	// verify
	c.Assert(*loc, check.DeepEquals, clusterApp.WithLiteralVersion("0.0.0+latest"))
}

func (s *S) TestGetsUpdatePackageByPattern(c *check.C) {
	var args localenv.TarballEnvironmentArgs
	updatePackagePattern := "app:2.0.2"

	// exercise
	loc, err := getUpdatePackage(args, updatePackagePattern, clusterApp)
	c.Assert(err, check.IsNil)

	// verify
	c.Assert(*loc, check.DeepEquals, clusterApp.WithLiteralVersion("2.0.2"))
}

func (s *S) TestGetsUpdatePackageFromTarballEnviron(c *check.C) {
	stateDir := createTarballEnviron(c)
	args := localenv.TarballEnvironmentArgs{
		StateDir: stateDir,
	}
	var emptyPackagePattern string

	// exercise
	loc, err := getUpdatePackage(args, emptyPackagePattern, clusterApp)
	c.Assert(err, check.IsNil)

	// verify
	c.Assert(*loc, check.DeepEquals, clusterApp.WithLiteralVersion("2.0.1"))
}

func createTarballEnviron(c *check.C) (stateDir string) {
	dir := c.MkDir()
	testServices := appservice.NewTestServices(c, appservice.WithDir(dir), appservice.WithDBFile(defaults.GravityDBFile))
	services := opsservice.SetupTestServices(c, opsservice.WithDir(dir), opsservice.WithTestServices(testServices))
	apptest.CreateApplication(apptest.AppRequest{
		App:      apptest.DefaultClusterApplication(clusterAppUpdate).Build(),
		Apps:     services.Apps,
		Packages: services.Packages,
	}, c)
	// Close the resources (incl. backend) to release the database file for reading
	services.Close()
	return dir
}

var (
	clusterApp = loc.Locator{
		Repository: defaults.SystemAccountOrg,
		Name:       "app",
		Version:    "2.0.0",
	}

	clusterAppUpdate = loc.Locator{
		Repository: defaults.SystemAccountOrg,
		Name:       "app",
		Version:    "2.0.1",
	}
)
