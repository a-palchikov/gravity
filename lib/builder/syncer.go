/*
Copyright 2018 Gravitational, Inc.

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

	libapp "github.com/gravitational/gravity/lib/app"
	"github.com/gravitational/gravity/lib/pack/localpack"
	"github.com/gravitational/gravity/lib/utils"

	"github.com/coreos/go-semver/semver"
	"github.com/sirupsen/logrus"
)

// Syncer synchronizes the local package cache from a (remote) repository
type Syncer interface {
	// Sync synchronizes the dependencies between a remote repository and the local cache directory.
	// app specifies the application to synchronize dependencies for.
	// intermediateRuntimes optionally provides the list of additional runtime application dependencies
	// to sync.
	// FIXME(dima): compute the application cache store on init
	Sync(ctx context.Context, cache Cache, app libapp.Application, apps libapp.Applications, intermediateRuntimes []semver.Version) error
}

// Cache describes the package context for the syncer
type Cache struct {
	Apps     libapp.Applications
	Pack     *localpack.PackageServer
	Logger   logrus.FieldLogger
	Progress utils.Progress
	Parallel int
}
