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
	"time"

	"github.com/gravitational/gravity/e/lib/environment"
	"github.com/gravitational/gravity/e/lib/ops"
	"github.com/gravitational/gravity/lib/constants"
	ossops "github.com/gravitational/gravity/lib/ops"

	"github.com/gravitational/trace"
)

func updateDownload(env *environment.Local, every string) error {
	operator, err := env.ClusterOperator()
	if err != nil {
		return trace.Wrap(err)
	}
	cluster, err := operator.GetLocalSite(context.TODO())
	if err != nil {
		return trace.Wrap(err)
	}
	// if "every" flag is provided, only update periodic updates status
	if every != "" {
		return trace.Wrap(setPeriodicUpdates(env, operator, *cluster, every))
	}
	update, err := operator.CheckForUpdate(cluster.Key())
	if err != nil && !trace.IsNotFound(err) {
		return trace.Wrap(err)
	}
	if update == nil {
		env.Println("No newer versions found")
		return nil
	}
	env.Printf("New version is available, downloading: %v\n", update)
	err = operator.DownloadUpdate(context.TODO(), ops.DownloadUpdateRequest{
		AccountID:   cluster.AccountID,
		SiteDomain:  cluster.Domain,
		Application: *update,
	})
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func setPeriodicUpdates(env *environment.Local, operator ops.Operator, cluster ossops.Site, every string) error {
	if every == constants.PeriodicUpdatesOff {
		err := operator.DisablePeriodicUpdates(context.TODO(), cluster.Key())
		if err != nil {
			return trace.Wrap(err)
		}
		env.Println("Periodic updates have been turned off")
		return nil
	}
	interval, err := time.ParseDuration(every)
	if err != nil {
		return trace.Wrap(err)
	}
	err = operator.EnablePeriodicUpdates(context.TODO(), ops.EnablePeriodicUpdatesRequest{
		AccountID:  cluster.AccountID,
		SiteDomain: cluster.Domain,
		Interval:   interval,
	})
	if err != nil {
		return trace.Wrap(err)
	}
	env.Println("Periodic updates have been turned on")
	return nil
}
