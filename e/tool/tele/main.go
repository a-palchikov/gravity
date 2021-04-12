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

package main

import (
	stdlog "log"
	"os"

	"github.com/gravitational/gravity/e/tool/tele/cli"
	"github.com/gravitational/gravity/tool/common"

	teleutils "github.com/gravitational/teleport/lib/utils"
	log "github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"

	// the following modules are imported just so their init methods are called
	_ "github.com/gravitational/gravity/e/lib/modules"
)

func main() {
	teleutils.InitLogger(teleutils.LoggingForCLI, log.InfoLevel)
	stdlog.SetOutput(log.StandardLogger().Writer())
	app := kingpin.New("tele", "Gravity tool for building and publishing cluster and application images (Enterprise edition).")
	if err := run(app); err != nil {
		log.WithError(err).Error("Command failed.")
		common.PrintError(err)
		os.Exit(255)
	}
}

func run(app *kingpin.Application) error {
	tele := cli.RegisterCommands(app)
	return common.ProcessRunError(cli.Run(tele))
}
