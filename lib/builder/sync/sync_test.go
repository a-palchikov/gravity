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
	"testing"

	apptest "github.com/gravitational/gravity/lib/app/service/test"
	"github.com/gravitational/gravity/lib/builder"
	"github.com/gravitational/gravity/lib/builder/suite"
	loctest "github.com/gravitational/gravity/lib/loc/test"
	"github.com/gravitational/gravity/lib/localenv"
	localenvtest "github.com/gravitational/gravity/lib/localenv/test"
	"github.com/gravitational/gravity/lib/utils"

	"github.com/coreos/go-semver/semver"
	"gopkg.in/check.v1"
)

func TestSyncer(t *testing.T) { check.TestingT(t) }

var (
	newLoc  = loctest.NewWithSystemRepository
	newLocs = loctest.NewLocsWithSystemRepository
)

func newEnviron(c *check.C) *localEnviron {
	env := localenvtest.New(c)
	return &localEnviron{LocalEnvironment: env}
}

func mustVer(v string) semver.Version {
	return *semver.New(v)
}

func newClusterBuilder(c *check.C, buildEnv *localEnviron, syncer builder.Syncer, opts ...func(*builder.Config)) *builder.ClusterBuilder {
	config := builder.Config{
		Logger:           utils.NewTestLogger(c),
		Env:              buildEnv.LocalEnvironment,
		SkipVersionCheck: true,
		Repository:       suite.Repository,
		Syncer:           syncer,
	}
	for _, opt := range opts {
		opt(&config)
	}
	b, err := builder.NewClusterBuilder(config)
	c.Assert(err, check.IsNil)
	return b
}

// Path returns the path of this environment's state directory.
// Implements suite.localBuilderContext
func (r *localEnviron) Path() string {
	return r.LocalEnvironment.StateDir
}

// CreateApp creates the application environment given by app and optional list
// of additional runtime applications.
// Implements suite.localBuilderContext
func (r *localEnviron) CreateApp(c *check.C, app apptest.App, runtimeApps ...apptest.App) {
	apptest.CreateApplicationDependencies(apptest.AppRequest{
		App:      app,
		Apps:     r.Apps,
		Packages: r.Packages,
	}, c)
	for _, runtimeApp := range runtimeApps {
		apptest.CreateApplication(apptest.AppRequest{
			App:      runtimeApp,
			Apps:     r.Apps,
			Packages: r.Packages,
		}, c)
	}
}

type localEnviron struct {
	*localenv.LocalEnvironment
}
