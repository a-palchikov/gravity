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

	libapp "github.com/gravitational/gravity/lib/app"
	"github.com/gravitational/gravity/lib/builder"
	"github.com/gravitational/gravity/lib/loc"
	"github.com/gravitational/gravity/lib/pack"
	"github.com/gravitational/gravity/lib/pack/localpack"
	"github.com/gravitational/gravity/lib/schema"
	"github.com/gravitational/gravity/lib/utils"
	"github.com/gravitational/trace"

	"github.com/coreos/go-semver/semver"
)

// Pack synchronizes local package cache with pack/apps services
type Pack struct {
	pack       pack.PackageService
	apps       libapp.Applications
	repository string
}

// NewPack creates a new syncer from provided pack and apps services
func NewPack(pack pack.PackageService, apps libapp.Applications, repository string) *Pack {
	return &Pack{
		pack:       pack,
		apps:       apps,
		repository: repository,
	}
}

// Sync pulls dependencies from the package/app service not available locally.
// app specifies the application to sync dependencies for.
// intermediateRuntimes optionally specifies additional runtime application versions to sync.
func (s *Pack) Sync(ctx context.Context, cache builder.Cache, app libapp.Application, filteredApps libapp.Applications, intermediateRuntimes []semver.Version) error {
	runtimeApp := func(version semver.Version) loc.Locator {
		return loc.Runtime.WithVersion(version)
	}
	for _, runtimeVersion := range intermediateRuntimes {
		cache.Progress.NextStep("Syncing packages for %v", runtimeVersion)
		// TODO(dmitri): distribution hub will not return the manifest correctly
		// for a recent version of the application due to compatibility issues.
		// Query the package and parse the manifest manually to be able to properly query
		// the runtime package
		runtimeApp, err := getApp(cache.Pack, s.pack, runtimeApp(runtimeVersion))
		if err != nil {
			return trace.Wrap(err)
		}
		err = libapp.VerifyDependencies(*runtimeApp, filteredApps, cache.Pack)
		if err != nil && !trace.IsNotFound(err) {
			return trace.Wrap(err)
		}
		if err == nil {
			cache.Logger.WithField("runtime-app", runtimeVersion).Info("Local package cache is up-to-date.")
			cache.Progress.NextStep("Local package cache is up-to-date for %v", runtimeVersion)
			continue
		}
		if err := s.sync(ctx, cache, *runtimeApp); err != nil {
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
	// Synchronize direct dependencies of the application
	if err := s.syncDeps(ctx, cache, app); err != nil {
		return trace.Wrap(err, "failed to sync dependencies for application %v", app)
	}
	return nil
}

func (s *Pack) sync(ctx context.Context, cache builder.Cache, app libapp.Application) error {
	puller := libapp.Puller{
		FieldLogger: cache.Logger,
		SrcPack:     s.pack,
		SrcApp:      s.apps,
		DstPack:     cache.Pack,
		DstApp:      cache.Apps,
		Parallel:    cache.Parallel,
		OnConflict:  libapp.OnConflictSkip,
	}
	err := puller.PullAppDeps(ctx, app)
	if err != nil {
		if utils.IsNetworkError(err) || trace.IsEOF(err) {
			return trace.ConnectionProblem(err, "failed to download "+
				"application dependencies from %v - please make sure the "+
				"repository is reachable: %v", s.repository, err)
		}
		return trace.Wrap(err)
	}
	return puller.PullAppOnly(ctx, app.Package)
}

func (s *Pack) syncDeps(ctx context.Context, cache builder.Cache, app libapp.Application) error {
	puller := libapp.Puller{
		FieldLogger: cache.Logger,
		SrcPack:     s.pack,
		SrcApp:      s.apps,
		DstPack:     cache.Pack,
		DstApp:      cache.Apps,
		Parallel:    cache.Parallel,
		OnConflict:  libapp.OnConflictSkip,
	}
	err := puller.PullAppDeps(ctx, app)
	if err != nil {
		if utils.IsNetworkError(err) || trace.IsEOF(err) {
			return trace.ConnectionProblem(err, "failed to download "+
				"application dependencies from %v - please make sure the "+
				"repository is reachable: %v", s.repository, err)
		}
		return trace.Wrap(err)
	}
	return nil
}

func getApp(localPack *localpack.PackageServer, pack pack.PackageService, loc loc.Locator) (*libapp.Application, error) {
	env, err := localPack.ReadPackageEnvelope(loc)
	if err != nil {
		if !trace.IsNotFound(err) {
			return nil, trace.Wrap(err)
		}
		// Re-read the package envelope from the remote store
		env, err = pack.ReadPackageEnvelope(loc)
		if err != nil {
			return nil, trace.Wrap(err)
		}
	}
	manifest, err := schema.ParseManifestYAMLNoValidate(env.Manifest)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &libapp.Application{
		Package:  loc,
		Manifest: *manifest,
	}, nil
}
