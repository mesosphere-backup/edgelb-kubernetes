package state

import (
	"fmt"
)

type VHost struct {
	Host   string           //FQDN of the VHost.
	Routes map[string]Route // All the routes supported by this VHost
}

type Route struct {
	Path     string             // The URL for the resource being accessed within the route.
	Backends map[string]Backend // All the backends that provide the resource associated with this URL.
}

type Backend struct {
	Address []string // An address for a backend is of the form IP:port
}

func NewVHost(Host string) (vhost *VHost) {
	vhost = &VHost{
		Host:  Host,
		Route: make(map[string]Route),
	}

	return
}

func NewRoute(Path string) (route *Route) {
	route = &Route{
		Path:     Path,
		Backends: make(map[string]Backend),
	}

	return
}
