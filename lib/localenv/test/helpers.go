package test

import (
	"time"

	"github.com/gravitational/gravity/lib/defaults"
	"github.com/gravitational/gravity/lib/localenv"

	"gopkg.in/check.v1"
)

// New returns a new local environment with the system repository
// precreated in the package store
func New(c *check.C) *localenv.LocalEnvironment {
	env, err := localenv.New(c.MkDir())
	c.Assert(err, check.IsNil)
	c.Assert(env.Packages.UpsertRepository(defaults.SystemAccountOrg, time.Time{}), check.IsNil)
	return env
}
