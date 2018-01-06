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
	Endpoints   []string
	URLs        []URL // A given service might be accessible from different URI.
}

type VHost struct {
	Host   string  // The FQDN used to access this VHost.
	Routes []Route // The different routes associated with this Vhost in order to access different services.
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
