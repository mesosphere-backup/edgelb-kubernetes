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

package config

type Ingress struct {
	IP       string
	Port     string
	Protocol string
}

type VHost struct {
	Frontend Ingress
	Host     string           //FQDN of the VHost.
	Routes   map[string]Route // All the routes supported by this VHost
}

type Route struct {
	Path        string  // The URL for the resource being accessed within the route.
	ReWritePath string  // The path that the backend expects.
	Service     Backend // Backends that serve this route.
}

type Backend struct {
	Endpoints []string // Represents all the replicas of the service. Each endpoint is of the form IP:port.
}

func NewVHost(Host string) (vhost *VHost) {
	vhost = &VHost{
		Host:   Host,
		Routes: make(map[string]Route),
	}

	return
}

func (route Route) String() string {
	return route.Path
}
