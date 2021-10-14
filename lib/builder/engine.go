/*
Copyright 2018-2020 Gravitational, Inc.

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
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"time"

	libapp "github.com/gravitational/gravity/lib/app"
	"github.com/gravitational/gravity/lib/app/service"
	blobfs "github.com/gravitational/gravity/lib/blob/fs"
	"github.com/gravitational/gravity/lib/constants"
	"github.com/gravitational/gravity/lib/defaults"
	"github.com/gravitational/gravity/lib/docker"
	"github.com/gravitational/gravity/lib/loc"
	"github.com/gravitational/gravity/lib/localenv"
	"github.com/gravitational/gravity/lib/pack"
	"github.com/gravitational/gravity/lib/pack/layerpack"
	"github.com/gravitational/gravity/lib/pack/localpack"
	"github.com/gravitational/gravity/lib/schema"
	"github.com/gravitational/gravity/lib/storage"
	"github.com/gravitational/gravity/lib/storage/keyval"
	"github.com/gravitational/gravity/lib/utils"

	"github.com/coreos/go-semver/semver"
	"github.com/docker/docker/pkg/archive"
	"github.com/ghodss/yaml"
	"github.com/gravitational/trace"
	"github.com/gravitational/version"
	"github.com/sirupsen/logrus"
)

// Config is the builder configuration
type Config struct {
	// StateDir is the configured builder state directory
	StateDir string
	// Insecure disables client verification of the server TLS certificate chain
	Insecure bool
	// Repository represents the source package repository
	Repository string
	// SkipVersionCheck allows to skip tele/runtime compatibility check
	SkipVersionCheck bool
	// Parallel is the builder's parallelism level
	Parallel int
	// Generator is used to generate installer
	Generator Generator
	// Syncer specifies the package syncer implementation
	Syncer Syncer
	// Env defines the build environment
	Env *localenv.LocalEnvironment
	// Progress allows builder to report build progress
	utils.Progress
	// UpgradeVia lists intermediate runtime versions to embed
	UpgradeVia []string
	// Logger specifies the logger to use
	Logger logrus.FieldLogger
}

// CheckAndSetDefaults validates builder config and fills in defaults
func (c *Config) CheckAndSetDefaults() error {
	if c.Env == nil {
		return trace.BadParameter("builder.Config.Env is required")
	}
	if c.Syncer == nil {
		return trace.BadParameter("builder.Config.Syncer is required")
	}
	if c.Parallel == 0 {
		c.Parallel = runtime.NumCPU()
	}
	if c.Generator == nil {
		c.Generator = &generator{}
	}
	if c.Progress == nil {
		c.Progress = utils.DiscardProgress
	}
	if c.Logger == nil {
		c.Logger = logrus.WithField(trace.Component, "builder")
	}
	return nil
}

// newEngine creates a new builder engine.
func newEngine(config Config) (*Engine, error) {
	if err := config.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	if err := checkBuildEnv(config.Logger); err != nil {
		return nil, trace.Wrap(err)
	}
	runtimeVersions, err := parseVersions(config.UpgradeVia)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	b := &Engine{
		Config:                      config,
		intermediateRuntimeVersions: runtimeVersions,
	}
	if err := b.initServices(); err != nil {
		b.Close()
		return nil, trace.Wrap(err)
	}
	return b, nil
}

// Engine is the builder engine that provides common functionality for building
// cluster and application images.
type Engine struct {
	// Config is the builder Engine configuration.
	Config
	// Dir is the directory where build-related data is stored.
	Dir string
	// Backend is the local backend.
	Backend storage.Backend
	// Packages is the layered package service with the local cache
	// directory serving as a 'read' layer and the temporary directory
	// as a 'read-write' layer.
	Packages pack.PackageService
	// Apps is the application service based on the layered package service.
	Apps libapp.Applications
	// intermediateRuntimeVersions lists intermediate runtime versions to embed in the resulting installer
	intermediateRuntimeVersions []semver.Version
}

// SelectRuntime picks an appropriate base image version for the cluster
// image that's being built
func (b *Engine) SelectRuntime(manifest *schema.Manifest) (*semver.Version, error) {
	runtime := manifest.Base()
	if runtime == nil {
		return nil, trace.NotFound("failed to determine application runtime")
	}
	switch runtime.Name {
	case constants.BaseImageName, defaults.Runtime:
	default:
		return nil, trace.BadParameter("unsupported base image %q, only %q is "+
			"supported as a base image", runtime.Name, constants.BaseImageName)
	}
	// If runtime version is explicitly set in the manifest, use it.
	if runtime.Version != loc.LatestVersion {
		b.Logger.WithField("version", runtime.Version).Info("Using pinned runtime version.")
		b.PrintSubStep("Will use base image version %s set in manifest", runtime.Version)
		return semver.NewVersion(runtime.Version)
	}
	// Otherwise, default to the version of this tele binary to ensure
	// compatibility.
	teleVersion, err := semver.NewVersion(version.Get().Version)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	b.Logger.WithField("version", teleVersion).Info("Selected runtime version based on tele version.")
	b.PrintSubStep("Will use base image version %s", teleVersion)
	return teleVersion, nil
}

// SyncPackageCache ensures that all system dependencies are present in
// the local cache directory for the specified application and the underlying list of intermediate runtime versions
func (b *Engine) SyncPackageCache(ctx context.Context, app libapp.Application) error {
	apps, err := b.Env.AppServiceLocal(localenv.AppConfig{ExcludeDeps: libapp.AppsToExclude(app.Manifest)})
	if err != nil {
		return trace.Wrap(err)
	}
	b.Logger.Infof("Synchronizing package cache with %v.", b.Repository)
	b.NextStep("Downloading dependencies from %v", b.Repository)
	return b.Syncer.Sync(ctx, b, app, apps, b.intermediateRuntimeVersions)
}

// VendorRequest combines vendoring parameters.
type VendorRequest struct {
	// SourceDir is the cluster or application image source directory.
	SourceDir string
	// VendorDir is the directory to perform vendoring in.
	VendorDir string
	// Manifest is the image manifest.
	Manifest *schema.Manifest
	// Vendor is parameters of the vendorer.
	Vendor service.VendorRequest
}

// Vendor vendors the application images in the provided directory and
// returns the compressed data stream with the application data
func (b *Engine) Vendor(ctx context.Context, req VendorRequest) (io.ReadCloser, error) {
	err := utils.CopyDirContents(req.SourceDir, filepath.Join(req.VendorDir, defaults.ResourcesDir))
	if err != nil {
		return nil, trace.Wrap(err)
	}
	manifestPath := filepath.Join(req.VendorDir, defaults.ResourcesDir, "app.yaml")
	data, err := yaml.Marshal(req.Manifest)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	err = ioutil.WriteFile(manifestPath, data, defaults.SharedReadMask)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	dockerClient, err := docker.NewClientFromEnv()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	vendorer, err := service.NewVendorer(service.VendorerConfig{
		DockerClient: dockerClient,
		ImageService: docker.NewDefaultImageService(),
		RegistryURL:  defaults.DockerRegistry,
		Packages:     b.Packages,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	vendorReq := req.Vendor
	vendorReq.ManifestPath = manifestPath
	vendorReq.ProgressReporter = b.Progress
	err = vendorer.VendorDir(ctx, req.VendorDir, vendorReq)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return archive.Tar(req.VendorDir, archive.Uncompressed)
}

// CreateApplication creates a Gravity application from the provided
// data in the local database
func (b *Engine) CreateApplication(data io.ReadCloser) (*libapp.Application, error) {
	progressC := make(chan *libapp.ProgressEntry)
	errorC := make(chan error, 1)
	err := b.Packages.UpsertRepository(defaults.SystemAccountOrg, time.Time{})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	op, err := b.Apps.CreateImportOperation(&libapp.ImportRequest{
		Source:    data,
		ProgressC: progressC,
		ErrorC:    errorC,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	// wait for the import to complete
	for range progressC {
	}
	err = <-errorC
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return b.Apps.GetImportedApplication(*op)
}

// WriteInstaller writes the provided installer tarball data to disk
func (b *Engine) WriteInstaller(data io.ReadCloser, outPath string) error {
	f, err := os.Create(outPath)
	if err != nil {
		return trace.Wrap(err)
	}
	_, err = io.Copy(f, data)
	return trace.Wrap(err)
}

// initServices initializes the builder backend, package and apps services
func (b *Engine) initServices() (err error) {
	b.Dir, err = ioutil.TempDir("", "build")
	if err != nil {
		return trace.Wrap(err)
	}
	defer func() {
		if err != nil {
			os.RemoveAll(b.Dir)
		}
	}()
	b.Backend, err = keyval.NewBolt(keyval.BoltConfig{
		Path: filepath.Join(b.Dir, defaults.GravityDBFile),
	})
	if err != nil {
		return trace.Wrap(err)
	}
	objects, err := blobfs.New(filepath.Join(b.Dir, defaults.PackagesDir))
	if err != nil {
		return trace.Wrap(err)
	}
	packages, err := localpack.New(localpack.Config{
		Backend:     b.Backend,
		UnpackedDir: filepath.Join(b.Dir, defaults.PackagesDir, defaults.UnpackedDir),
		Objects:     objects,
	})
	if err != nil {
		return trace.Wrap(err)
	}
	b.Packages = layerpack.New(b.Env.Packages, packages)
	b.Apps, err = b.Env.AppServiceLocal(localenv.AppConfig{
		Packages: b.Packages,
	})
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// checkVersion makes sure that the tele version is compatible with the selected
// runtime version.
func (b *Engine) checkVersion(runtimeVersion *semver.Version) error {
	if b.SkipVersionCheck {
		return nil
	}
	if err := checkVersion(runtimeVersion, b.Logger); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// collectUpgradeDependencies computes and returns a set of package dependencies for each
// configured intermediate runtime version.
func (b *Engine) collectUpgradeDependencies(manifest *schema.Manifest) (result *libapp.Dependencies, err error) {
	apps, err := b.Env.AppServiceLocal(localenv.AppConfig{})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	result = &libapp.Dependencies{}
	for _, runtimeVersion := range b.intermediateRuntimeVersions {
		app, err := apps.GetApp(loc.Runtime.WithVersion(runtimeVersion))
		if err != nil {
			return nil, trace.Wrap(err)
		}
		req := libapp.GetDependenciesRequest{
			App:  *app,
			Apps: apps,
			Pack: b.Env.Packages,
		}
		dependencies, err := libapp.GetDependencies(req)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		v := schema.IntermediateVersion{
			Version: runtimeVersion,
			Dependencies: schema.Dependencies{
				Packages: toSchemaPackageDependencies(dependencies.Packages),
				Apps:     toSchemaAppDependencies(dependencies.Apps),
			},
		}
		manifest.SystemOptions.Dependencies.IntermediateVersions = append(
			manifest.SystemOptions.Dependencies.IntermediateVersions, v)
		result.Packages = append(result.Packages, dependencies.Packages...)
		result.Apps = append(result.Apps, append(dependencies.Apps, *app)...)
	}
	return result, nil
}

// Close cleans up build environment
func (b *Engine) Close() error {
	var errors []error
	if b.Env != nil {
		errors = append(errors, b.Env.Close())
	}
	if b.Backend != nil {
		errors = append(errors, b.Backend.Close())
	}
	if b.Dir != "" {
		errors = append(errors, os.RemoveAll(b.Dir))
	}
	if b.Progress != nil {
		b.Progress.Stop()
	}
	return trace.NewAggregate(errors...)
}

// app returns the application value with the specified runtime version
// set as the base
func app(loc loc.Locator, manifest schema.Manifest) libapp.Application {
	return libapp.Application{
		Package:  loc,
		Manifest: manifest,
	}
}

func runtimeApp(version semver.Version) loc.Locator {
	return loc.Runtime.WithVersion(version)
}

func parseVersions(versions []string) (result []semver.Version, err error) {
	result = make([]semver.Version, 0, len(versions))
	for _, version := range versions {
		runtimeVersion, err := semver.NewVersion(version)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		result = append(result, *runtimeVersion)
	}
	return result, nil
}

func toSchemaPackageDependencies(packages []pack.PackageEnvelope) (result []schema.Dependency) {
	result = make([]schema.Dependency, 0, len(packages))
	for _, pkg := range packages {
		result = append(result, schema.Dependency{Locator: pkg.Locator})
	}
	return result
}

func toSchemaAppDependencies(apps []libapp.Application) (result []schema.Dependency) {
	result = make([]schema.Dependency, 0, len(apps))
	for _, app := range apps {
		result = append(result, schema.Dependency{Locator: app.Package})
	}
	return result
}
