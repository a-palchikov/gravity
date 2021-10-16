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

package test

import (
	"strings"

	"github.com/gravitational/gravity/lib/defaults"
	"github.com/gravitational/gravity/lib/loc"
)

// NewLocsWithSystemRepository transforms the specified list of package
// references to the list of package locators.
// locs are assumed to be in the format accepted by NewWithSystemRepository
func NewLocsWithSystemRepository(locs ...string) (result []loc.Locator) {
	for _, l := range locs {
		result = append(result, NewWithSystemRepository(l))
	}
	return result
}

// NewWithSystemRepository converts the specified package reference
// given as name:version to the full package locator defaulting to
// the system repository
func NewWithSystemRepository(nameVersion string) loc.Locator {
	parts := strings.Split(nameVersion, ":")
	return loc.Locator{
		Repository: defaults.SystemAccountOrg,
		Name:       parts[0],
		Version:    parts[1],
	}
}
