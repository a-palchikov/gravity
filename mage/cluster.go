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
	"path/filepath"

	"github.com/gravitational/magnet"
	"github.com/gravitational/trace"
	"github.com/magefile/mage/mg"
)

type Cluster mg.Namespace

// Gravity builds the reference gravity cluster image.
func (Cluster) Gravity(ctx context.Context) (err error) {
	mg.CtxDeps(ctx, Mkdir(consistentStateDir()), Mkdir(consistentBinDir()),
		Build.Go, Package.Telekube)

	m := root.Target("cluster:gravity")
	defer func() { m.Complete(err) }()

	return buildGravityClusterImage(ctx, m, "")
}

// GravityVia builds the reference gravity cluster image with an intermediate
// upgrade step for version given with upgradeVia
func (Cluster) GravityVia(ctx context.Context, upgradeVia string) (err error) {
	mg.CtxDeps(ctx, Mkdir(consistentStateDir()), Mkdir(consistentBinDir()),
		Build.Go, Package.Telekube)

	m := root.Target("cluster:gravity-via")
	defer func() { m.Complete(err) }()

	if err := importIntermediatePackages(ctx, m, upgradeVia); err != nil {
		return trace.Wrap(err)
	}

	return buildGravityClusterImage(ctx, m, upgradeVia)
}

func importIntermediatePackages(ctx context.Context, t *magnet.MagnetTarget, upgradeVia string) (err error) {
	_, err = t.Exec().Run(ctx, "hack/import-upgrade-via", upgradeVia, buildVersion)
	return trace.Wrap(err)
}

func buildGravityClusterImage(ctx context.Context, t *magnet.MagnetTarget, upgradeVia string) (err error) {
	args := []string{
		"--debug",
		"build",
		"assets/telekube/resources/app.yaml",
		"--overwrite",
		"--version", buildVersion,
		"--state-dir", consistentStateDir(),
		"--skip-version-check",
		"--output", filepath.Join(consistentBuildDir(), "gravity.tar"),
	}
	if upgradeVia != "" {
		args = append(args, "--upgrade-via", upgradeVia)
	}
	_, err = t.Exec().SetEnv("GRAVITY_K8S_VERSION", k8sVersion).Run(ctx,
		filepath.Join(consistentBinDir(), "tele"), args...)
	return trace.Wrap(err)
}

// Hub builds the reference hub cluster image.
func (Cluster) Hub(ctx context.Context) (err error) {
	mg.CtxDeps(ctx, Mkdir(consistentStateDir()), Mkdir(consistentBinDir()),
		Build.Go, Package.Telekube)

	m := root.Target("cluster:hub")
	defer func() { m.Complete(err) }()

	_, err = m.Exec().SetEnv("GRAVITY_K8S_VERSION", k8sVersion).Run(context.TODO(),
		filepath.Join(consistentBinDir(), "tele"),
		"--debug",
		"build",
		"assets/opscenter/resources/app.yaml",
		"-f",
		"--version", buildVersion,
		"--state-dir", consistentStateDir(),
		"--skip-version-check",
		"-o", filepath.Join(consistentBuildDir(), "hub.tar"),
	)
	return trace.Wrap(err)
}

// Wormhole builds the reference gravity cluster image based on wormhole networking.
func (Cluster) Wormhole(ctx context.Context) (err error) {
	mg.CtxDeps(ctx, Mkdir(consistentStateDir()), Mkdir(consistentBinDir()),
		Build.Go, Package.Telekube)

	m := root.Target("cluster:wormhole")
	defer func() { m.Complete(err) }()

	_, err = m.Exec().SetEnv("GRAVITY_K8S_VERSION", k8sVersion).Run(context.TODO(),
		filepath.Join(consistentBinDir(), "tele"),
		"--debug",
		"build",
		"assets/wormhole/resources/app.yaml",
		"-f",
		"--version", buildVersion,
		"--state-dir", consistentStateDir(),
		"--skip-version-check",
		"-o", filepath.Join(consistentBuildDir(), "wormhole.tar"),
	)
	return trace.Wrap(err)
}
