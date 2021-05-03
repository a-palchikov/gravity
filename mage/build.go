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
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	"github.com/gravitational/trace"

	"github.com/magefile/mage/mg"
)

type Build mg.Namespace

func (Build) All() {
	mg.SerialDeps(Build.Go)
}

func buildBoxName() string {
	return fmt.Sprint("gravity-build:", buildVersion)
}

func (r Build) Go() error {
	if runtime.GOOS == "darwin" {
		return r.Darwin()
	}
	return r.Linux()
}

// Linux builds Go Linux binaries using consistent build environment.
func (Build) Linux() (err error) {
	mg.Deps(Build.BuildContainer, Build.Selinux)

	m := root.Target("build:go")
	defer func() { m.Complete(err) }()

	packages := []string{"github.com/gravitational/gravity/tool/gravity", "github.com/gravitational/gravity/tool/tele"}

	err = m.GolangBuild().
		SetGOOS("linux").
		SetGOARCH("amd64").
		SetEnv("GO111MODULE", "on").
		SetMod("vendor").
		AddTag("selinux", "selinux_embed").
		SetBuildContainer(fmt.Sprint("gravity-build:", buildVersion)).
		SetOutputPath(consistentContainerBinDir()).
		AddLDFlags(buildFlags()).
		Build(context.TODO(), packages...)

	return trace.Wrap(err)
}

// LinuxOsArch builds Go Linux binaries using consistent build environment and outputs
// files with os/arch suffix.
func (Build) LinuxOsArch() (err error) {
	mg.Deps(Build.BuildContainer, Build.Selinux)

	m := root.Target("build:go")
	defer func() { m.Complete(err) }()

	packages := []string{"github.com/gravitational/gravity/tool/gravity"}

	err = m.GolangBuild().
		SetGOOS("linux").
		SetGOARCH("amd64").
		SetEnv("GO111MODULE", "on").
		SetMod("vendor").
		AddTag("selinux", "selinux_embed").
		SetBuildContainer(fmt.Sprint("gravity-build:", buildVersion)).
		SetOutputPath(osArchBinDir("linux", "amd64")).
		AddLDFlags(buildFlags()).
		Build(context.TODO(), packages...)

	return trace.Wrap(err)
}

// Darwin builds Go binaries on the Darwin platform (doesn't support cross compile).
func (Build) Darwin() (err error) {
	m := root.Target("build:darwin")
	defer func() { m.Complete(err) }()

	if runtime.GOOS != "darwin" {
		return trace.BadParameter("Cross-compile not currently supported, darwin builds need to be run on darwin OS")
	}

	err = m.GolangBuild().
		SetGOOS("darwin").
		SetGOARCH("amd64").
		SetEnv("GO111MODULE", "on").
		SetMod("vendor").
		SetOutputPath(consistentBinDir()).
		AddLDFlags(buildFlags()).
		Build(context.TODO(), "github.com/gravitational/gravity/tool/gravity", "github.com/gravitational/gravity/tool/tele")

	return trace.Wrap(err)
}

// BuildContainer creates a docker container as a consistent Go environment to use for software builds.
func (Build) BuildContainer() (err error) {
	m := root.Target("build:buildContainer")
	defer func() { m.Complete(err) }()

	err = m.DockerBuild().
		AddTag(buildBoxName()).
		SetPull(true).
		SetBuildArg("GOLANG_VER", golangVersion).
		SetBuildArg("PROTOC_VER", grpcProtocVersion).
		SetBuildArg("PROTOC_PLATFORM", grpcProtocPlatform).
		SetBuildArg("GOGO_PROTO_TAG", grpcGoGoTag).
		SetBuildArg("GRPC_GATEWAY_TAG", grpcGatewayTag).
		SetBuildArg("GOLANGCI_VER", golangciVersion).
		SetBuildArg("UID", fmt.Sprint(os.Getuid())).
		SetBuildArg("GID", fmt.Sprint(os.Getgid())).
		SetDockerfile("build.assets/Dockerfile.buildx").
		Build(context.TODO(), "./build.assets")

	return trace.Wrap(err)
}

// Selinux builds internal selinux code
func (Build) Selinux(ctx context.Context) (err error) {
	mg.Deps(Build.SelinuxPolicy)

	m := root.Target("build:selinux")
	defer func() { m.Complete(err) }()

	uptodate := IsUpToDate("lib/system/selinux/internal/policy/policy_embed.go",
		"lib/system/selinux/internal/policy/policy.go",
		"lib/system/selinux/internal/policy/assets",
	)
	if uptodate {
		return nil
	}

	_, err = m.Exec().Run(ctx, "make", "-C", "lib/system/selinux")
	return trace.Wrap(err)
}

func (Build) SelinuxPolicy(ctx context.Context) (err error) {
	m := root.Target("build:selinuxpolicy")
	defer func() { m.Complete(err) }()

	tmpDir, err := ioutil.TempDir("", "build-selinux")
	if err != nil {
		return trace.ConvertSystemError(err)
	}
	defer os.RemoveAll(tmpDir)

	cachePath := root.inBuildDir("apps", fmt.Sprint("selinux.", pkgSelinux.version, ".tar.gz"))

	if _, err := os.Stat(cachePath); err == nil {
		m.SetCached(true)
		_, err := m.Exec().
			// TODO(dima): have selinux makefile output to a subdirectory per
			// supported OS distribution instead of hardcoding it
			Run(ctx, "tar", "xf", cachePath, "-C", "lib/system/selinux/internal/policy/assets/centos")
		return trace.Wrap(err)
	}

	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return trace.Wrap(trace.ConvertSystemError(err))
	}

	_, err = m.Exec().SetWD(tmpDir).Run(ctx,
		"git",
		"clone",
		selinuxRepo,
		"--branch", selinuxBranch,
		"--depth=1",
		"./",
	)
	if err != nil {
		return trace.Wrap(err)
	}

	_, err = m.Exec().SetWD(tmpDir).Run(ctx,
		"git",
		"submodule",
		"update",
		"--init",
		"--recursive",
	)
	if err != nil {
		return trace.Wrap(err)
	}

	outputDir := filepath.Join(tmpDir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return trace.Wrap(trace.ConvertSystemError(err))
	}

	_, err = m.Exec().SetWD(tmpDir).Run(ctx, "make", "BUILDBOX_INSTANCE=")
	if err != nil {
		return trace.Wrap(err)
	}

	_, err = m.Exec().
		Run(ctx, "tar", "czf", cachePath,
			"-C", outputDir,
			"gravity.pp.bz2",
			"container.pp.bz2",
			"gravity.statedir.fc.template",
		)
	return trace.Wrap(err)
}
