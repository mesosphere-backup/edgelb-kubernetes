package lb

import (
	"fmt"

	// Edgelb-k8s packages
	"edgelb-k8s/pkg/lb/config"
	"edgelb-k8s/pkg/state"
)

type LoadBalancer interface {
	AddVHost(vhost state.VHost) error
	RemoveVHost(vhost state.VHost) error
	ExposeService(vhost state.VHost, service state.Service) error
	RemoveService(vhost state.VHost, service state.Service) error
}

// The remote backend which is a physical representation of this LoadBalancer.
type LoadBalancerBackend interface {
	ConfigureVHost(vhost config.VHost) error
	UnconfigureVHost(vhost config.VHost) error
}

type DefaultLoadBalancer struct {
	lbBackend LoadBalancerBackend
	vhosts    map[string]config.VHost
}

func NewDefaultLoadBalancer(lbBackend LoadBalancerBackend) (lb *DefaultLoadBalancer) {
	lb = &DefaultLoadBalancer{
		vhosts:    make(map[string]config.VHost),
		lbBackend: lbBackend,
	}

	return
}

func (lb *DefaultLoadBalancer) checkVHost(vhost string) bool {
	_, ok := lb.vhosts[vhost]
	return ok
}

func (lb *DefaultLoadBalancer) AddVHost(vhost state.VHost) (err error) {
	if lb.checkVHost(vhost.Host) {
		err = fmt.Errorf("Cannot add an existing VHost:%s", vhost.Host)
		return
	}

	lb.vhosts[vhost.Host] = *config.NewVHost(vhost.Host)

	return
}

func (lb *DefaultLoadBalancer) RemoveVHost(vhost state.VHost) (err error) {
	if !lb.checkVHost(vhost.Host) {
		err = fmt.Errorf("Cannot remove a non-existent VHost:%s", vhost.Host)
		return
	}

	delete(lb.vhosts, vhost.Host)

	return
}

func (lb *DefaultLoadBalancer) ExposeService(vhost state.VHost, service state.Service) (err error) {
	lbVHost, ok := lb.vhosts[vhost.Host]
	if !ok {
		err = fmt.Errorf("Cannot expose service:%s to non-existent VHost:%s", service, vhost.Host)
		return
	}

	// Add endpoints from this service to the given VHost.
	for _, url := range service.URLs {
		if url.Host == lbVHost.Host {
			route := config.Route{Path: url.Path}
			service := &route.Service

			for _, endpoint := range service.Endpoints {
				service.Endpoints = append(service.Endpoints, endpoint)
			}
			lbVHost.Routes[route.String()] = route

			// Write the `lbVHost` object back to the `map` so that the state gets updated.
			lb.vhosts[lbVHost.Host] = lbVHost
		}
	}

	return

}

func (lb *DefaultLoadBalancer) RemoveService(vhost state.VHost, service state.Service) (err error) {
	lbVHost, ok := lb.vhosts[vhost.Host]
	if !ok {
		err = fmt.Errorf("Cannot remove service:%s to non-existent VHost:%s", service, vhost.Host)
		return
	}

	// Remove endpoints from this service to the given VHost.
	for _, url := range service.URLs {
		if url.Host == lbVHost.Host {
			if _, ok := lbVHost.Routes[url.Path]; !ok {
				err = fmt.Errorf("Cannot remove service:%s to non-existent path %s on VHost:%s", service, url.Host, url.Path)
				return
			}

			// Assumption is that only a single service would be exposed on a given path.
			delete(lbVHost.Routes, url.Path)

			// Write the `lbVHost` object back to the `map` so that the state gets updated.
			lb.vhosts[lbVHost.Host] = lbVHost
		}
	}

	return
}
