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
	"context"
	"io"
	"io/ioutil"
	"os"

	libapp "github.com/gravitational/gravity/lib/app"
	"github.com/gravitational/gravity/lib/archive"
	"github.com/gravitational/gravity/lib/builder"
	"github.com/gravitational/gravity/lib/defaults"
	"github.com/gravitational/gravity/lib/loc"
	"github.com/gravitational/gravity/lib/localenv"
	"github.com/gravitational/trace"

	"github.com/coreos/go-semver/semver"
)

// S3 synchronizes local package cache with S3 bucket
type S3 struct {
	// hub provides access to runtimes stored in S3 bucket
	hub HubGetter
}

// NewS3 returns a syncer that syncs packages with S3 bucket
func NewS3(hub HubGetter) *S3 {
	return &S3{
		hub: hub,
	}
}

// HubGetter defines a way to pull a package from a remote hub
type HubGetter interface {
	// Get returns application installer tarball of the specified version
	Get(loc.Locator) (io.ReadCloser, error)
}

// Sync makes sure that local cache has all required dependencies for the
// selected runtime.
// intermediateRuntimes optionally specifies additional runtime application versions to sync.
//
// This implementation does not attempt to sync the application's direct dependencies since
// they are never available in the oss hub and instead should already be in the local package cache.
func (s *S3) Sync(ctx context.Context, cache builder.Cache, app libapp.Application, filteredApps libapp.Applications, intermediateRuntimes []semver.Version) error {
	if app.Manifest.Base() == nil {
		return trace.BadParameter("runtime version unspecified in manifest")
	}
	runtimeApp := func(version string) loc.Locator {
		return loc.Locator{
			Repository: defaults.SystemAccountOrg,
			Name:       defaults.TelekubePackage,
			Version:    version,
		}
	}
	for _, runtimeVersion := range intermediateRuntimes {
		// FIXME(dima): apps service has the list of application dependencies filtered according
		// to the manifest configuration. Have this configuration be implicit and not require an
		// additional application service instance (e.g. use engine.Env.Apps directly)
		//
		// When working with an S3 backend, it is not possible to verify whether the local
		// cache already has the dependenies w/o also downloading them, so just sync
		// unconditionally
		runtimeApp := runtimeApp(runtimeVersion.String())
		if err := s.sync(ctx, cache, runtimeApp); err != nil {
			return trace.Wrap(err, "failed to sync packages for runtime version %v", runtimeVersion)
		}
	}
	err := libapp.VerifyDependencies(app, filteredApps, cache.Pack)
	if err != nil && !trace.IsNotFound(err) {
		return trace.Wrap(err)
	}
	if err == nil {
		cache.Logger.WithField("app", app.String()).Info("Local package cache is up-to-date.")
		cache.Progress.NextStep("Local package cache is up-to-date for %v", app)
		return nil
	}
	if err := s.sync(ctx, cache, runtimeApp(app.Manifest.Base().Version)); err != nil {
		return trace.Wrap(err, "failed to sync packages for runtime version %v", app.Manifest.Base().Version)
	}
	return nil
}

func (s *S3) sync(ctx context.Context, cache builder.Cache, app loc.Locator) error {
	unpackedDir, err := ioutil.TempDir("", "runtime-unpacked")
	if err != nil {
		return trace.Wrap(err)
	}
	defer os.RemoveAll(unpackedDir)

	err = s.download(unpackedDir, app)
	if err != nil {
		return trace.Wrap(err)
	}
	tarballEnv, err := localenv.NewTarballEnvironment(localenv.TarballEnvironmentArgs{
		StateDir: unpackedDir,
	})
	if err != nil {
		return trace.Wrap(err)
	}
	defer tarballEnv.Close()

	// sync packages between unpacked tarball directory and package cache
	tarballApp, err := tarballEnv.Apps.GetApp(app)
	if err != nil {
		return trace.Wrap(err)
	}
	puller := libapp.Puller{
		FieldLogger: cache.Logger,
		SrcPack:     tarballEnv.Packages,
		SrcApp:      tarballEnv.Apps,
		DstPack:     cache.Pack,
		DstApp:      cache.Apps,
		Parallel:    cache.Parallel,
		Upsert:      true,
	}
	return puller.PullApp(ctx, tarballApp.Package)
}

func (s *S3) download(path string, loc loc.Locator) error {
	tarball, err := s.hub.Get(loc)
	if err != nil {
		return trace.Wrap(err)
	}
	defer tarball.Close()
	// unpack the downloaded tarball
	err = archive.Extract(tarball, path)
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}
