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

package mage

import (
	"context"
	"fmt"
	"os"

	"github.com/gravitational/magnet"
	"github.com/gravitational/trace"
	"github.com/magefile/mage/mg"
)

type Test mg.Namespace

func (Test) All() {
	//mg.SerialDeps(Build.Go, Build.BuildContainer)
	mg.Deps(Test.Unit, Test.Lint)
}

// Lint runs golangci linter against the repo.
func (Test) Lint() (err error) {
	mg.Deps(Build.BuildContainer)

	m := root.Target("test:lint")
	defer func() { m.Complete(err) }()

	m.Printlnf("Running golangci-lint")
	m.Println("  Linter: ", golangciVersion)

	wd, _ := os.Getwd()
	cacheDir, err := root.Config.AbsCacheDir()
	if err != nil {
		return trace.Wrap(err)
	}
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return trace.Wrap(trace.ConvertSystemError(err))
	}

	err = m.DockerRun().
		SetRemove(true).
		SetUID(fmt.Sprint(os.Getuid())).
		SetGID(fmt.Sprint(os.Getgid())).
		AddVolume(magnet.DockerBindMount{
			Source:      wd,
			Destination: "/gopath/src/github.com/gravitational/gravity",
			Readonly:    true,
			Consistency: "cached",
		}).
		AddVolume(magnet.DockerBindMount{
			Source:      cacheDir,
			Destination: "/cache",
			Consistency: "cached",
		}).
		SetEnv("XDG_CACHE_HOME", "/cache").
		SetEnv("GOCACHE", "/cache/go").
		Run(context.TODO(), buildBoxName(),
			"/usr/bin/dumb-init",
			"bash", "-c",
			"golangci-lint run /gopath/src/github.com/gravitational/gravity/... --config /gopath/src/github.com/gravitational/gravity/.golangci.yml",
		)

	return trace.Wrap(err)
}

// Unit runs unit tests with the race detector enabled.
func (Test) Unit() (err error) {
	mg.Deps(Build.BuildContainer)

	m := root.Target("test:unit")
	defer func() { m.Complete(err) }()

	m.Println("Running unit tests")

	err = m.GolangTest().
		SetRace(true).
		SetBuildContainer(buildBoxName()).
		SetEnv("GO111MODULE", "on").
		SetMod("vendor").
		Test(context.TODO(), "./...")
	return
}
