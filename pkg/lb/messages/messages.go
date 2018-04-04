// Copyright (c) 2018 Mesosphere, Inc
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package messages

import (
	"github.com/mesosphere/edgelb-k8s/pkg/lb/config"
)

type SyncMsg struct{}

type ConfigVHostsMsg struct {
	VHosts []config.VHost
}

type ConfigVHostMsg struct {
	VHost config.VHost
}

// Note that the `RemoveVHostMsg` is identical to the `ConfigVHostsMsg`. The
// semantic here is that the controller presents the load-balancer with all the
// existing VHosts, minus the VHost that got removed.
type RemoveVHostMsg struct {
	VHosts []config.VHost
}

type UnConfigVHostMsg struct {
	VHost config.VHost
}
