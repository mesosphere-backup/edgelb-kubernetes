package lb

import (
	"fmt"

	// Edgelb-k8s packages
	LBState "pkg/lb/state"
	"pkg/state"
)

type LoadBalancer interface {
	AddVHost(vhost state.VHost) error
	RemoveVHost(vhost state.VHost) error
	ExposeService(vhost state.VHost, service state.Service) error
	RemoveService(vhost state.VHost, service state.Service) error
}

type DefaultLoadBalancer struct {
	vhosts map[string]LBState.VHost
}

func NewDefaultLoadBalancer() (lb *DefaultLoadBalancer) {
	lb := &DefaultLoadBalancer{
		vhosts: make(map[string]LBState.VHost),
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

	lb.vhosts[vhost.Host] = *LBState.NewVHost(vhost.Host)

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
	var lbVHost LBState.VHost
	if lbVHost, ok := lb.vhosts[vhost.Host]; !ok {
		err = fmt.Errorf("Cannot expose service:%s to non-existent VHost:%s", service, vhost.Host)
		return
	}

	// Add endpoints from this service to the given VHost.
	for _, url := range service.URLs {
		if url.Host == lbVHost.Host {
			if _, ok := lbVHost.Routes[url.Path]; !ok {
				err = fmt.Errorf("Cannot expose service:%s to non-existent path %s on VHost:%s", service, url.Host, url.Path)
				return
			}

			for endpoint := range service.Endpoints {
				lbVHost.Routes[url.Path][endpoint] = LBState.Address{Address: endpoint}
			}

			// Write the `lbVHost` object back to the `map` so that the state gets updated.
			lb.vhosts[lbVHost.Host] = lbVHost
		}
	}

	return

}

func (lb *DefaultLoadBalancer) RemoveService(vhost state.VHost, service state.Service) (err error) {
	var lbVHost LBState.VHost
	if lbVHost, ok := lb.vhosts[vhost.Host]; !ok {
		err = fmt.Errorf("Cannot remove service:%s to non-existent VHost:%s", service, vhost.Host)
		return
	}

	// Add endpoints from this service to the given VHost.
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
