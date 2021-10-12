// Copyright 2021 Gravitational Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"context"

	"github.com/gravitational/gravity/e/lib/builder"
	basebuilder "github.com/gravitational/gravity/lib/builder"
	"github.com/gravitational/gravity/lib/defaults"
	"github.com/gravitational/gravity/lib/localenv"
	"github.com/gravitational/gravity/lib/localenv/credentials"
	"github.com/gravitational/gravity/tool/tele/cli"
	"github.com/gravitational/trace"

	"github.com/sirupsen/logrus"
)

// buildParameters extends CLI parameters for open-source version of tele build
type buildParameters struct {
	// BuildParameters is the build parameters from open-source
	cli.BuildParameters
	// RemoteSupport is an optional remote Ops Center address
	RemoteSupportAddress string
	// RemoteSupport is an optional remote Ops Center token
	RemoteSupportToken string
	// CACertPath is an optional path to CA certificate installer will use
	CACertPath string
	// EncryptionKey is an optional key used to encrypt installer packages at rest
	EncryptionKey string
	// Credentials is the optional credentials set on the CLI
	Credentials *credentials.Credentials
}

func (p buildParameters) repository() (string, error) {
	if p.Credentials != nil {
		return p.Credentials.URL, nil
	}
	credentialsService, err := credentials.New(credentials.Config{
		LocalKeyStoreDir: p.StateDir,
	})
	if err != nil {
		return "", trace.Wrap(err)
	}
	// with no static credentials set use the cluster we're logged into
	credentials, err := credentialsService.Current()
	if err == nil {
		return credentials.URL, nil
	}
	// use the default one if no cluster context
	if trace.IsNotFound(err) {
		return defaults.DistributionOpsCenter, nil
	}
	return "", trace.Wrap(err)
}

func (p buildParameters) generator() (basebuilder.Generator, error) {
	return builder.NewGenerator(builder.Config{
		RemoteSupportAddress: p.RemoteSupportAddress,
		RemoteSupportToken:   p.RemoteSupportToken,
		CACertPath:           p.CACertPath,
		EncryptionKey:        p.EncryptionKey,
	})
}

func buildClusterImage(ctx context.Context, params buildParameters) error {
	logger := logrus.WithField(trace.Component, "builder")
	repository, err := params.repository()
	if err != nil {
		return trace.Wrap(err)
	}
	env, err := params.newBuildEnviron(repository, logger)
	if err != nil {
		return trace.Wrap(err)
	}
	pack, err := env.PackageService(repository)
	if err != nil {
		return trace.Wrap(err)
	}
	apps, err := env.AppService(repository, localenv.AppConfig{})
	if err != nil {
		return trace.Wrap(err)
	}
	syncer := basebuilder.NewPackSyncer(pack, apps, repository)
	generator, err := params.generator()
	if err != nil {
		return trace.Wrap(err)
	}
	config := params.BuilderConfig(ctx)
	config.Generator = generator
	config.Repository = repository
	config.Env = env
	config.Syncer = syncer
	config.Logger = logger
	clusterBuilder, err := basebuilder.NewClusterBuilder(config)
	if err != nil {
		return trace.Wrap(err)
	}
	defer clusterBuilder.Close()
	return clusterBuilder.Build(ctx, basebuilder.ClusterRequest{
		SourcePath: params.SourcePath,
		OutputPath: params.OutPath,
		Overwrite:  params.Overwrite,
		BaseImage:  params.BaseImage,
		Vendor:     params.Vendor,
	})
}

func (p buildParameters) newBuildEnviron(repository string, logger logrus.FieldLogger) (*localenv.LocalEnvironment, error) {
	// if state directory was specified explicitly, it overrides
	// both cache directory and config directory as it's used as
	// a special case only for building from local packages
	if p.StateDir != "" {
		logger.Infof("Using package cache from %v.", p.StateDir)
		return localenv.NewLocalEnvironment(localenv.LocalEnvironmentArgs{
			StateDir:         p.StateDir,
			LocalKeyStoreDir: p.StateDir,
			Insecure:         p.Insecure,
			Credentials:      p.Credentials,
		})
	}
	// otherwise use default locations for cache / key store
	cacheDir, err := basebuilder.EnsureCacheDir(repository)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	logger.Infof("Using package cache from %v.", cacheDir)
	return localenv.NewLocalEnvironment(localenv.LocalEnvironmentArgs{
		StateDir:    cacheDir,
		Insecure:    p.Insecure,
		Credentials: p.Credentials,
	})
}
