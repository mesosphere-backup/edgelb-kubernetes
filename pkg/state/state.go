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

func (service ServiceName) String() string {
	return fmt.Sprintf("%s%s", service.Namespace, service.Name)
}

func (url URL) String() string {
	return fmt.Sprintf("%s%s", url.Host, url.Path)
}

func (route Route) String() string {
	return route.Path
}

func (endpoint Endpoint) String() string {
	return fmt.Sprintf("%s@%s", endpoint.ServiceName, endpoint.Address)
}
