// +build selinux_embed

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

package policy

import (
	"io"
	"embed"
)

//go:embed assets/centos/container.pp.bz2 assets/centos/gravity.pp.bz2 assets/centos/gravity.statedir.fc.template
var contents embed.FS

// Policy contains the SELinux policy.
var Policy = policyFS{fs: contents}

// Open returns the reader to the file with the specified path
func (r policyFS) Open(path string) (io.ReadCloser, error) {
	return r.fs.Open(path)
}

type policyFS struct{
	fs embed.FS
}
