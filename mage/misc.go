/*
Copyright 2020 Gravitational, Inc.
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
	"fmt"
	"os"
	"path/filepath"

	"github.com/gravitational/trace"

	"github.com/magefile/mage/target"
)

// Clean cleans up the build directory.
func Clean() (err error) {
	m := root.Target("build:clean")
	defer func() { m.Complete(err) }()

	return trace.Wrap(os.RemoveAll(root.buildDir))
}

// Env outputs the list of imported environment variables
func Env() (err error) {
	for k, v := range root.Config.ImportEnv {
		fmt.Println(k, ":", v)
	}

	return
}

// IsUpToDate returns true iff all of the (source, sources...) are older or the same
// age as dst.
func IsUpToDate(dst string, source string, sources ...string) (uptodate bool) {
	var files []string
	for _, source := range append(sources, source) {
		filepath.Walk(source, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if fi.Mode().IsRegular() {
				files = append(files, path)
			}
			return nil
		})
	}
	newer, err := target.Path(dst, files...)
	return err == nil && !newer
}
