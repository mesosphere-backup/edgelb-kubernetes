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

package state

import (
	"fmt"
)

// Controller representation of a k8s service. This is primarily required to track the end-points.
type ServiceName struct {
	Namespace string
	Name      string
}

type Service struct {
	ServiceName // Fields representing a service.
	Endpoints   []Endpoint
	URLs        map[string]URL // A given service might be accessible from different URL.
}

type Endpoint struct {
	ServiceName        // The service to which this endpoint belongs.
	Address     string // A string representation of endpoint of the form IP:port.
}

type VHost struct {
	Host   string           // The FQDN used to access this VHost.
	Routes map[string]Route // The different routes associated with this Vhost in order to access different services.
}

type URL struct {
	Host string // This is the `Host` field of a `VHost`.
	Path string // The path within the given host.
}

type Route struct {
	Path        string
	ServiceName // The service associated with this Route.
}

func NewVHost(host string) (vhost *VHost) {
	vhost = &VHost{
		Host:   host,
		Routes: make(map[string]Route),
	}

	return
}

func NewService(id ServiceName) (service *Service) {
	service = &Service{
		ServiceName: id,
		Endpoints:   make([]Endpoint, 0),
		URLs:        make(map[string]URL),
	}
	return
}

func (vhost VHost) String() string {
	return vhost.Host
}

func (service ServiceName) String() string {
	return fmt.Sprintf("%s/%s", service.Namespace, service.Name)
}

func (url URL) String() string {
	return fmt.Sprintf("%s/%s", url.Host, url.Path)
}

func (route Route) String() string {
	return route.Path
}

func (endpoint Endpoint) String() string {
	return fmt.Sprintf("%s@%s", endpoint.ServiceName, endpoint.Address)
}
