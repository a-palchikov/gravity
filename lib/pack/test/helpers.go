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
	"fmt"
	"io"

	"github.com/gravitational/gravity/lib/compare"
	"github.com/gravitational/gravity/lib/loc"
	"github.com/gravitational/gravity/lib/pack"

	"gopkg.in/check.v1"
)

// ListPackages dumps packages in the specified service to w
func ListPackages(packages pack.PackageService, w io.Writer, c *check.C) {
	repositories, err := packages.GetRepositories()
	c.Assert(err, check.IsNil)
	for _, r := range repositories {
		envs, err := packages.GetPackages(r)
		c.Assert(err, check.IsNil)
		for _, p := range envs {
			env, err := packages.ReadPackageEnvelope(p.Locator)
			c.Assert(err, check.IsNil)
			fmt.Fprintln(w, "Package: ", env.Locator, " with labels: ", env.RuntimeLabels)
		}
	}
}

// VerifyPackages ensures that the specified package service contains
// the expected packages.
// No assumptions are made about other packages in the service
func VerifyPackages(packages pack.PackageService, expected []loc.Locator, c *check.C) {
	repositories, err := packages.GetRepositories()
	c.Assert(err, check.IsNil)

	var result []loc.Locator
	for _, repository := range repositories {
		packages, err := packages.GetPackages(repository)
		c.Assert(err, check.IsNil)
		result = append(result, locators(packages)...)
	}

	c.Assert(packagesByName(result), compare.SortedSliceEquals, packagesByName(expected))
}

func VerifyPackagesWithLabels(packages pack.PackageService, expected PackagesWithLabels, c *check.C) {
	var obtained PackagesWithLabels
	pack.ForeachPackage(packages, func(e pack.PackageEnvelope) error {
		labels := e.RuntimeLabels
		if labels == nil {
			// To compensate for package envelopes with nil labels
			// when comparing
			labels = make(map[string]string)
		}
		obtained = append(obtained, PackageWithLabels{
			loc:    e.Locator,
			labels: labels,
		})
		return nil
	})
	c.Assert(obtained, compare.SortedSliceEquals, expected)
}

func locators(envelopes []pack.PackageEnvelope) []loc.Locator {
	out := make([]loc.Locator, 0, len(envelopes))
	for _, env := range envelopes {
		out = append(out, env.Locator)
	}
	return out
}

type packagesByName []loc.Locator

func (r packagesByName) Len() int           { return len(r) }
func (r packagesByName) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
func (r packagesByName) Less(i, j int) bool { return r[i].String() < r[j].String() }

// NewPackage creates a new package with optional labels
func NewPackage(s string, labels ...string) PackageWithLabels {
	if len(labels)%2 != 0 {
		panic("number of labels must be even")
	}
	m := make(map[string]string)
	for i := 0; i < len(labels); i += 2 {
		m[labels[i]] = labels[i+1]
	}
	return PackageWithLabels{loc: loc.MustParseLocator(s), labels: m}
}

func (r PackagesWithLabels) Len() int { return len(r) }
func (r PackagesWithLabels) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}
func (r PackagesWithLabels) Less(i, j int) bool {
	return r[i].loc.String() < r[j].loc.String()
}

type PackagesWithLabels []PackageWithLabels

type PackageWithLabels struct {
	loc    loc.Locator
	labels map[string]string
}
