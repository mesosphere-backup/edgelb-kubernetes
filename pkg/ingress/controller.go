package ingress

import (
	"errors"
	"fmt"
	"log"
	"reflect"
	"time"

	// Actor
	"github.com/AsynkronIT/protoactor-go/actor"

	// Ingress controller
	"edgelb-k8s/pkg/state"

	// RxGo
	"github.com/reactivex/rxgo/iterable"
	"github.com/reactivex/rxgo/observable"
	"github.com/reactivex/rxgo/observer"

	// K8s
	V1api "k8s.io/api/core/v1"
	V1Beta1api "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/listers/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
)

const (
	ADD    = iota
	UPDATE = iota
	DEL    = iota
)

type Operation struct {
	Op int
}

type k8sIngressMsg struct {
	Op      Operation
	Ingress V1Beta1api.Ingress
}

type k8sServiceMsg struct {
	Op      Operation
	Service V1api.Service
}

type k8sEndpointsMsg struct {
	Op        Operation
	Endpoints V1api.Endpoints
}

type ingressRuleMsg struct {
	Op          Operation
	Namespace   string
	IngressRule V1Beta1api.IngressRule
}

type syncMsg struct{}

// Used to add/del/update a `Host` on this controller.
type hostMsg struct {
	Op   Operation
	Host string
}

// Used to add/del/update a `state.Service` on this controller.
type serviceMsg struct {
	Op      Operation
	Service state.ServiceName
}

// Used to add/del/update a service endpoint on this controller.
type endpointMsg struct {
	Op       Operation
	Service  state.ServiceName
	Endpoint string
}

type Controller interface {
	Start() error
}

type controller struct {
	pid              *actor.PID // PID of the controller.
	lb               *actor.PID // PID of the load-balancer.
	si               informers.SharedInformerFactory
	endpoints        v1.EndpointsLister        // All the endpoints that are availabe in a k8s cluster.
	ingressResources v1beta1.IngressLister     // Ingress resource that define the config for the controller.
	services         map[string]*state.Service // Services for which the controller is asked to setup ingress.
	vhosts           map[string]*state.VHost
	//Observable channels
	ingressMsgs   chan interface{}
	serviceMsgs   chan interface{}
	endpointsMsgs chan interface{}
}

func NewController(clientset *kubernetes.Clientset, loadBalancer *actor.PID) (ctrl Controller) {
	resyncPeriod := 30 * time.Minute
	si := informers.NewSharedInformerFactory(clientset, resyncPeriod)

	ingressCtrl := controller{
		si:               si,
		lb:               loadBalancer,
		endpoints:        si.Core().V1().Endpoints().Lister(),
		ingressResources: si.Extensions().V1beta1().Ingresses().Lister(),
		services:         make(map[string]*state.Service),
		vhosts:           make(map[string]*state.VHost),
		ingressMsgs:      make(chan interface{}),
		serviceMsgs:      make(chan interface{}),
		endpointsMsgs:    make(chan interface{}),
	}

	ctrl = &ingressCtrl

	return
}

func (ctrl *controller) Start() (err error) {
	// Store the PID and spawn the actor.
	ctrl.pid = actor.Spawn(actor.FromInstance(ctrl))

	// Start the k8s client.
	ctrl.si.Start(wait.NeverStop)

	endPointsCache := ctrl.si.Core().V1().Endpoints().Informer()
	servicesCache := ctrl.si.Core().V1().Services().Informer()
	ingressRulesCache := ctrl.si.Extensions().V1beta1().Ingresses().Informer()

	// Now let's start the controller
	syncCh := make(chan struct{})
	defer close(syncCh)
	if !cache.WaitForCacheSync(syncCh, endPointsCache.HasSynced, servicesCache.HasSynced, ingressRulesCache.HasSynced) {
		err = errors.New("Timed out waiting for caches to sync")
		return

	}

	// Sync done. Reconcile
	err = ctrl.sync()
	if err != nil {
		return
	}

	// At this point we have learnt all the ingress rules that existed at the
	// API server till this point. We need to send a message to the actor to
	// start syncing these ingress rules with the load-balancer.

	// Setup observers so that we can process the different k8s messages we are
	// interested in.
	sink := observer.Observer{
		NextHandler: func(item interface{}) {
			log.Printf("Sending a resource update of type:%s to ingress controller", reflect.TypeOf(item))
			switch item.(type) {
			case k8sIngressMsg:
				msg, _ := item.(k8sIngressMsg)
				ctrl.pid.Tell(&msg)
			case k8sServiceMsg:
				msg, _ := item.(k8sServiceMsg)
				ctrl.pid.Tell(&msg)
			case k8sEndpointsMsg:
				msg, _ := item.(k8sEndpointsMsg)
				ctrl.pid.Tell(&msg)
			}
		},

		// Register a handler for any emitted error.
		ErrHandler: func(err error) {
			log.Printf("Encountered error: %v\n", err)
		},

		// Register a handler when a stream is completed.
		DoneHandler: func() {
			log.Println("Done!")
		},
	}

	k8sIngressSource, ingressErr := iterable.New(ctrl.ingressMsgs)
	if ingressErr != nil {
		log.Fatalf("Couldn't create ingress message sink %s", ingressErr)
	}

	k8sServiceSource, serviceErr := iterable.New(ctrl.serviceMsgs)
	if serviceErr != nil {
		log.Fatalf("Couldn't create service message sink %s", serviceErr)
	}

	k8sEndpointsSource, endpointsErr := iterable.New(ctrl.endpointsMsgs)
	if endpointsErr != nil {
		log.Fatalf("Couldn't create endpoints message sink %s", endpointsErr)
	}

	observable.From(k8sIngressSource).Subscribe(sink)
	observable.From(k8sServiceSource).Subscribe(sink)
	observable.From(k8sEndpointsSource).Subscribe(sink)

	// NOTE: Between the sync and the setting up of watchers for endpoints,
	// services and ingress resourcess there is a time window in which an
	// ingress resource is added which we will end up missing. We will need to
	// revisit to try and determine how to reduce this window. Ofcourse any
	// ingress resource that we miss, we can learn if the operator deletes it
	// and recreates it.

	// Add watchers for endpoints.
	log.Printf("Ingress controller setting up `v1.Endpoints` watchers...")
	ctrl.si.Core().V1().Endpoints().Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				log.Printf("Received an add event for `v1.Endpoints` resource")
				ctrl.endpointsMsgs <- k8sEndpointsMsg{
					Op:        Operation{Op: ADD},
					Endpoints: *(obj.(*V1api.Endpoints)),
				}
			},
			DeleteFunc: func(obj interface{}) {
				log.Printf("Received a delete event for `v1.Endpoints` resource")
				ctrl.endpointsMsgs <- k8sEndpointsMsg{
					Op:        Operation{Op: DEL},
					Endpoints: *(obj.(*V1api.Endpoints)),
				}
			},
		},
	)

	// Add watchers for services.
	log.Printf("Ingress controller setting up `v1.Services` watchers...")
	ctrl.si.Core().V1().Services().Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				log.Printf("Received an add event for `v1.Services` resource")
				ctrl.serviceMsgs <- k8sServiceMsg{
					Op:      Operation{Op: ADD},
					Service: *(obj.(*V1api.Service)),
				}
			},
			UpdateFunc: func(old interface{}, new interface{}) {
				log.Printf("Received an update event for `v1.Services` resource")
				ctrl.serviceMsgs <- k8sServiceMsg{
					Op:      Operation{Op: UPDATE},
					Service: *(new.(*V1api.Service)),
				}
			},
			DeleteFunc: func(obj interface{}) {
				log.Printf("Received a delete event for `v1.Services` resource")
				ctrl.serviceMsgs <- k8sServiceMsg{
					Op:      Operation{Op: DEL},
					Service: *(obj.(*V1api.Service)),
				}
			},
		},
	)

	// Add watchers for ingress
	log.Printf("Ingress controller setting up `v1.beta1.Ingress` watchers...")
	ctrl.si.Extensions().V1beta1().Ingresses().Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				log.Printf("Received an add event for `v1.beta1.Ingress` resource")
				ctrl.ingressMsgs <- k8sIngressMsg{
					Op:      Operation{Op: ADD},
					Ingress: *(obj.(*V1Beta1api.Ingress)),
				}
			},
			UpdateFunc: func(old interface{}, new interface{}) {
				log.Printf("Received an update event for `v1.beta1.Ingress` resource")
				ctrl.ingressMsgs <- k8sIngressMsg{
					Op:      Operation{Op: UPDATE},
					Ingress: *(new.(*V1Beta1api.Ingress)),
				}
			},
			DeleteFunc: func(obj interface{}) {
				log.Printf("Received a delete event for `v1.beta1.Ingress` resource")
				ctrl.ingressMsgs <- k8sIngressMsg{
					Op:      Operation{Op: DEL},
					Ingress: *(obj.(*V1Beta1api.Ingress)),
				}
			},
		},
	)

	<-wait.NeverStop

	return
}

func (ctrl *controller) Receive(ctx actor.Context) {
	switch ctx.Message().(type) {
	case *k8sIngressMsg:
		ingressMsg, _ := ctx.Message().(*k8sIngressMsg)
		switch operation := ingressMsg.Op.Op; operation {
		case ADD, UPDATE:
			log.Printf("Received an ADD/UPDATE `*k8sIngressMsg.")
			ctrl.ingressCreateAndUpdate(ingressMsg.Ingress)
		case DEL:
			log.Printf("Received a DEL `*k8sIngressMsg.")
			ctrl.ingressDeleted(ingressMsg.Ingress)
		default:
			log.Printf("Undefined operation %d requested on `k8sIngressMsg`", operation)
		}
	case *ingressRuleMsg:
		ingressRuleMsg, _ := ctx.Message().(*ingressRuleMsg)
		switch operation := ingressRuleMsg.Op.Op; operation {
		case ADD, UPDATE:
			log.Printf("Received an ADD/UPDATE `*ingressRuleMsg.")
			host := ctrl.ingressRuleCreateAndUpdate(ingressRuleMsg.Namespace, ingressRuleMsg.IngressRule)
			log.Printf("Need to ADD/UPDATE host:%s on load-balancer.", host)
			// Tell the controller to process this host.
			ctrl.pid.Tell(&hostMsg{Op: Operation{Op: ADD}, Host: host})
		case DEL:
			log.Printf("Received a DEL `*ingressRuleMsg.")
			host := ctrl.ingressRuleDeleted(ingressRuleMsg.Namespace, ingressRuleMsg.IngressRule)
			// Tell the controller to delete this host.
			log.Printf("Need to DEL host:%s from load-balancer.", host)
			ctrl.pid.Tell(&hostMsg{Op: Operation{Op: DEL}, Host: host})
		default:
			log.Printf("Undefined operation %d requested on `IngressRuleMsg`", operation)
		}
	case *k8sServiceMsg, *k8sEndpointsMsg:
		var id state.ServiceName
		var op Operation
		switch ctx.Message().(type) {
		case *k8sServiceMsg:
			service := (ctx.Message().(*k8sServiceMsg)).Service
			id = state.ServiceName{Namespace: service.GetNamespace(), Name: service.GetName()}
			op = (ctx.Message().(*k8sServiceMsg)).Op
		case *k8sEndpointsMsg:
			endpoints := (ctx.Message().(*k8sEndpointsMsg)).Endpoints
			id = state.ServiceName{Namespace: endpoints.GetNamespace(), Name: endpoints.GetName()}
			op = (ctx.Message().(*k8sEndpointsMsg)).Op
		}
		switch op.Op {
		case ADD, UPDATE:
			log.Printf("Received an ADD/UPDATE `*k8sServiceMsg for:%s", id)
			hosts := ctrl.updateServiceEndpoints(id)
			for _, host := range hosts {
				// Tell the controller to process this host.
				ctrl.pid.Tell(&hostMsg{Op: Operation{Op: ADD}, Host: host})
			}
		case DEL:
			log.Printf("Received a DEL `*k8sServiceMsg for:%s", id)
			hosts := ctrl.deleteService(id)
			for _, host := range hosts {
				// Tell the controller to delete this host.
				ctrl.pid.Tell(&hostMsg{Op: Operation{Op: DEL}, Host: host})
			}
		default:
			log.Printf("Undefined operation %d requested on `k8ServiceMsg/k8sEndpointsMsg`", op)
		}
	case *hostMsg:
		hostMsg, _ := ctx.Message().(*hostMsg)
		switch operation := hostMsg.Op.Op; operation {
		case ADD, UPDATE:
			log.Printf("Will send update for host:%s to the load-balancer", hostMsg.Host)
		case DEL:
			log.Printf("Will delete host:%s from the load-balancer", hostMsg.Host)
		default:
			log.Printf("Undefined operation %d requested on `hostMsg` handler", operation)
		}
	case *syncMsg:
		// Inform the load-balancer about all the VHosts and their associated backends.

	default:
		log.Printf("Unsopported message received by ingress controller:%s", reflect.TypeOf(ctx.Message()))
	}
}

// Walks through the cached list of ingress resources and builds the internal
// state of `VHosts` and `service endpoints associated with the `VHosts`.
func (ctrl *controller) sync() (err error) {
	// Get all the ingress resources.
	ingressResources, err := ctrl.ingressResources.List(labels.Everything())

	if err != nil {
		return
	}

	for _, ingressResource := range ingressResources {
		// Process each of the ingress rule.
		for _, ingressRule := range ingressResource.Spec.Rules {
			ctrl.ingressRuleCreateAndUpdate(ingressResource.GetNamespace(), ingressRule)
		}
	}

	return
}

func (ctrl *controller) endpointCreateUpdateAndDelete(obj interface{}) {
	endpoint, ok := obj.(*V1api.Endpoints)
	if !ok {
		log.Printf("Expected an object of type `*v1.Endpoint`, but got object of type: %s", reflect.TypeOf(obj))
		return
	}

	service := state.ServiceName{
		Name:      endpoint.GetName(),
		Namespace: endpoint.GetNamespace(),
	}

	serviceMsg := &serviceMsg{Op: Operation{Op: UPDATE}, Service: service}

	// We won't do anything specific to this endpoint. We will just ask the Service to recreate all the endpoints
	// belonging to this service if the service is actually being exposed.
	ctrl.pid.Tell(serviceMsg)
}

func (ctrl *controller) serviceCreateUpdateAndDelete(obj interface{}, Op Operation) {
	service, ok := obj.(*V1api.Service)
	if !ok {
		log.Printf("Expected an object of type `*v1.Service`, but got object of type: %s", reflect.TypeOf(obj))
		return
	}

	id := state.ServiceName{
		Name:      service.GetName(),
		Namespace: service.GetNamespace(),
	}

	serviceMsg := &serviceMsg{Op: Op, Service: id}

	// We won't do anything specific to this endpoint. We will just ask the Service to recreate all the endpoints
	// belonging to this service if the service is actually being exposed.
	ctrl.pid.Tell(serviceMsg)

}

func (ctrl *controller) ingressCreateAndUpdate(ingress V1Beta1api.Ingress) {
	namespace := ingress.GetNamespace()

	it, _ := iterable.New(ingress.Spec.Rules)

	// Process all the rules.
	observable.From(it).Subscribe(observer.Observer{
		// For every VHost that we get, register it with the load-balancer.
		NextHandler: func(item interface{}) {
			ingressRule := item.(*V1Beta1api.IngressRule)
			// Ask the controller to process this rule.
			ctrl.pid.Tell(&ingressRuleMsg{
				Op:          Operation{Op: ADD},
				Namespace:   namespace,
				IngressRule: *ingressRule})
		},
	})
}

func (ctrl *controller) ingressDeleted(ingress V1Beta1api.Ingress) {
	it, _ := iterable.New(ingress.Spec.Rules)

	// Process all the rules.
	observable.From(it).Subscribe(observer.Observer{
		// For every VHost that we get, register it with the load-balancer.
		NextHandler: func(item interface{}) {
			ingressRule := item.(*V1Beta1api.IngressRule)
			// Ask the controller to process this rule.
			ctrl.pid.Tell(&ingressRuleMsg{
				Op:          Operation{Op: DEL},
				Namespace:   ingress.GetNamespace(),
				IngressRule: *ingressRule})
		},
	})
}

// Create a `VHost` based on an `IngressRule`.
// Returns the host added/deleted in this updated.
func (ctrl *controller) ingressRuleCreateAndUpdate(namespace string, rule V1Beta1api.IngressRule) (host string) {
	// If a VHost already exists delete it since we will be re-creating it here.
	if _, ok := ctrl.vhosts[rule.Host]; ok {
		delete(ctrl.vhosts, rule.Host)
	}

	vhost := &state.VHost{Host: rule.Host}

	for _, path := range rule.HTTP.Paths {
		url := state.URL{Host: rule.Host, Path: path.Path}
		id := state.ServiceName{Namespace: namespace, Name: path.Backend.ServiceName}

		// Store the association of the service with the URI
		if _, ok := ctrl.services[id.String()]; !ok {
			ctrl.services[id.String()] = &state.Service{ServiceName: id}
		}

		ctrl.services[id.String()].URLs[url.String()] = url

		ctrl.updateServiceEndpoints(id)

		// Append the route to the VHost.
		route := state.Route{Path: url.Path, ServiceName: id}
		vhost.Routes[route.String()] = route
	}

	ctrl.vhosts[vhost.Host] = vhost

	host = vhost.Host

	return
}

// Delete a `Vhost` based on an IngressRule.
// Returns the `host` that got deleted.
func (ctrl *controller) ingressRuleDeleted(namespace string, rule V1Beta1api.IngressRule) (host string) {
	host = rule.Host

	// If a VHost already exists delete it since we will be re-creating it here.
	if _, ok := ctrl.vhosts[rule.Host]; ok {
		delete(ctrl.vhosts, rule.Host)
	}

	for _, path := range rule.HTTP.Paths {
		url := state.URL{Host: rule.Host, Path: path.Path}
		id := state.ServiceName{Namespace: namespace, Name: path.Backend.ServiceName}
		service, ok := ctrl.services[id.String()]
		// Delete any association of the service with this host
		if ok {
			delete(service.URLs, url.String())
		}
	}

	return
}

// Takes a `Service` and updates the endpoints of the service.
// Returns the affected vhosts.
func (ctrl *controller) updateServiceEndpoints(id state.ServiceName) (host []string) {
	service, ok := ctrl.services[id.String()]

	if !ok {
		// We don't have a VHost corresponding to this servcie so we don't need to do anything.
		log.Printf("Found a service(%s) that is not part of any ingress reousrce, hence skipping", id)
		return
	}

	// Since we might actually be adding/removing existing endpoints to the service,
	// remove the existing endpoints from the service before adding new ones.
	service.Endpoints = nil

	// Look at the service name, and get the corresponding endpoints for this service name.
	endpoints, err := ctrl.endpoints.Endpoints(service.Namespace).Get(service.Name)
	if err != nil {
		for _, endpoint := range endpoints.Subsets {
			for _, address := range endpoint.Addresses {
				for _, port := range endpoint.Ports {
					service.Endpoints = append(
						service.Endpoints,
						state.Endpoint{
							ServiceName: id,
							Address:     fmt.Sprintf("%s:%d", address.IP, port.Port),
						})
				}
			}

			for _, address := range endpoint.NotReadyAddresses {
				for _, port := range endpoint.Ports {
					service.Endpoints = append(
						service.Endpoints,
						state.Endpoint{
							ServiceName: id,
							Address:     fmt.Sprintf("%s:%d", address.IP, port.Port),
						})
				}
			}

		}
	} else {
		log.Printf("Unable to retrieve the endpoints for service:%s, error:%s", *service, err)
	}

	for vhost, _ := range service.URLs {
		host = append(host, vhost)
	}

	return
}

func (ctrl *controller) deleteService(id state.ServiceName) (host []string) {
	service, ok := ctrl.services[id.String()]

	if !ok {
		// We don't have a VHost corresponding to this servcie so we don't need to do anything.
		return
	}

	for vhost, _ := range service.URLs {
		host = append(host, vhost)
	}

	delete(ctrl.services, id.String())

	return
}
