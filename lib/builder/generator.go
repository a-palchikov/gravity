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
	"github.com/gravitational/gravity/lib/app"
	"github.com/gravitational/gravity/lib/schema"
)

// Generator defines a method for generating standalone installers
type Generator interface {
	// NewInstallerRequest returns a new request to generate an installer
	// for the specified application
	NewInstallerRequest(*Engine, schema.Manifest, app.Application) (*app.InstallerRequest, error)
}

type generator struct{}

// NewInstallerRequest returns a request to build an installer for the specified application
func (g *generator) NewInstallerRequest(engine *Engine, _ schema.Manifest, application app.Application) (*app.InstallerRequest, error) {
	return &app.InstallerRequest{
		Application: application.Package,
	}, nil
}
