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
package service

import (
	"path/filepath"

	"github.com/gravitational/gravity/lib/blob/fs"
	"github.com/gravitational/gravity/lib/defaults"
	"github.com/gravitational/gravity/lib/helm"
	"github.com/gravitational/gravity/lib/pack"
	"github.com/gravitational/gravity/lib/pack/localpack"
	"github.com/gravitational/gravity/lib/storage"
	"github.com/gravitational/gravity/lib/storage/keyval"
	"github.com/jonboulle/clockwork"

	"github.com/mailgun/timetools"
	"gopkg.in/check.v1"
)

// TestServices groups services relevant in package/application tests
type TestServices struct {
	Backend  storage.Backend
	Packages pack.PackageService
	Apps     *Applications
}

// NewTestServices creates a new set of test services
func NewTestServices(c *check.C, opts ...TestServiceOption) *TestServices {
	t := testServices{
		dbfile: "bolt.db",
		clock:  clockwork.NewFakeClock(),
	}
	for _, opt := range opts {
		opt(&t)
	}
	if t.stateDir == "" {
		t.stateDir = c.MkDir()
	}

	backend, err := keyval.NewBolt(keyval.BoltConfig{
		Path: filepath.Join(t.stateDir, t.dbfile),
	})
	c.Assert(err, check.IsNil)

	objects, err := fs.New(t.stateDir)
	c.Assert(err, check.IsNil)

	packages, err := localpack.New(localpack.Config{
		Backend:     backend,
		UnpackedDir: filepath.Join(t.stateDir, defaults.UnpackedDir),
		Objects:     objects,
		Clock: &timetools.FreezedTime{
			CurrentTime: t.clock.Now(),
		},
		DownloadURL: "https://ops.example.com",
	})
	c.Assert(err, check.IsNil)

	charts, err := helm.NewRepository(helm.Config{
		Packages: packages,
		Backend:  backend,
	})
	c.Assert(err, check.IsNil)

	apps, err := New(Config{
		Backend:  backend,
		StateDir: filepath.Join(t.stateDir, defaults.ImportDir),
		Packages: packages,
		Charts:   charts,
	})
	c.Assert(err, check.IsNil)

	return &TestServices{
		Backend:  backend,
		Packages: packages,
		Apps:     apps,
	}
}

type testServices struct {
	stateDir string
	dbfile   string
	clock    clockwork.FakeClock
}

type TestServiceOption func(*testServices)

func WithClock(clock clockwork.FakeClock) TestServiceOption {
	return func(r *testServices) {
		r.clock = clock
	}
}

// WithDir sets the state directory to dir
func WithDir(dir string) TestServiceOption {
	return func(r *testServices) {
		r.stateDir = dir
	}
}

// WithDBFile sets the filename of the database file
func WithDBFile(filename string) TestServiceOption {
	return func(r *testServices) {
		r.dbfile = filename
	}
}

// Close releases the internal resources.
// Implements io.Closer
func (r *TestServices) Close() error {
	return r.Backend.Close()
}
