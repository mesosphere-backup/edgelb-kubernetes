package ingress

import (
	"fmt"
	"log"
	"reflect"
	"time"

	// Actor
	"github.com/AsynkronIT/protoactor-go/actor"

	// Ingress controller
	"github.com/asridharan/edgelb-k8s/pkg/state"

	// RxGo
	"github.com/reactivex/rxgo/iterable"
	"github.com/reactivex/rxgo/observable"
	"github.com/reactivex/rxgo/observer"

	// K8s
	V1api "k8s.io/api/core/v1"
	V1Beta1api "k8s.io/api/extensions/v1beta1"
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

type IngressMsg struct {
	Op          Operation
	Namespace   string
	IngressRule V1Beta1api.IngressRule
}

// Used to add/del/update a `state.VHost` on this controller.
type VHostMsg struct {
	Op    Operation
	VHost state.VHost
}

// Used to add/del/update a `state.Service` on this controller.
type ServiceMsg struct {
	Op      Operation
	Service state.ServiceName
}

// Used to add/del/update a service endpoint on this controller.
type EndpointMsg struct {
	Op       Operation
	Service  state.ServiceName
	Endpoint string
}

type Controller interface {
	Start()
}

type controller struct {
	pid              *actor.PID // PID of the controller.
	si               informers.SharedInformerFactory
	endpoints        v1.EndpointsLister        // All the endpoints that are availabe in a k8s cluster.
	ingressResources v1beta1.IngressLister     // Ingress resource that define the config for the controller.
	services         map[string]*state.Service // Services for which the controller is asked to setup ingress.
	vhosts           map[string]*state.VHost
}

func NewController(clientset *kubernetes.Clientset) (ctrl Controller, err error) {
	resyncPeriod := 30 * time.Minute
	si := informers.NewSharedInformerFactory(clientset, resyncPeriod)

	ingressCtrl := controller{
		si:               si,
		endpoints:        si.Core().V1().Endpoints().Lister(),
		ingressResources: si.Extensions().V1beta1().Ingresses().Lister(),
		services:         make(map[string]*state.Service),
		vhosts:           make(map[string]*state.VHost),
	}

	ctrl = &ingressCtrl

	// Add watchers for endpoints.
	si.Core().V1().Endpoints().Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: ingressCtrl.endpointCreateUpdateAndDelete,
			UpdateFunc: func(old interface{}, new interface{}) {
				ingressCtrl.endpointCreateUpdateAndDelete(new)
			},
			DeleteFunc: ingressCtrl.endpointCreateUpdateAndDelete,
		},
	)

	// Add watchers for services.
	si.Core().V1().Services().Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				ingressCtrl.serviceCreateUpdateAndDelete(obj, Operation{Op: ADD})
			},
			UpdateFunc: func(old interface{}, new interface{}) {
				ingressCtrl.serviceCreateUpdateAndDelete(new, Operation{Op: UPDATE})
			},
			DeleteFunc: func(obj interface{}) {
				ingressCtrl.serviceCreateUpdateAndDelete(obj, Operation{Op: DEL})
			},
		},
	)

	// Add watchers for ingress
	si.Extensions().V1beta1().Ingresses().Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: ingressCtrl.ingressCreateAndUpdate,
			UpdateFunc: func(old interface{}, new interface{}) {
				ingressCtrl.ingressCreateAndUpdate(new)
			},
			DeleteFunc: ingressCtrl.ingressDeleted,
		},
	)

	return
}

func (ctrl *controller) Start() {
	// Store the PID and spawn the actor.
	ctrl.pid = actor.Spawn(actor.FromInstance(ctrl))

	// Start the watchers.
	ctrl.si.Start(wait.NeverStop)
	<-wait.NeverStop
}

func (ctrl *controller) Receive(ctx actor.Context) {
	switch ctx.Message().(type) {
	case *IngressMsg:
		ingressMsg, _ := ctx.Message().(*IngressMsg)
		switch operation := ingressMsg.Op.Op; operation {
		case ADD, UPDATE:
			ctrl._ingressCreateAndUpdate(ingressMsg.Namespace, ingressMsg.IngressRule)
		case DEL:
			ctrl._ingressDeleted(ingressMsg.Namespace, ingressMsg.IngressRule)
		default:
			log.Printf("Undefined operation %d requested on `IngressMsg`", operation)
		}
	case *ServiceMsg:
		serviceMsg, _ := ctx.Message().(*ServiceMsg)
		switch operation := serviceMsg.Op.Op; operation {
		case ADD, UPDATE:
			ctrl.updateServiceEndpoints(serviceMsg.Service)
		case DEL:
			ctrl.deleteService(serviceMsg.Service)
		default:
			log.Printf("Undefined operation %d requested on `IngressMsg`", operation)
		}
	default:
		log.Printf("Unsopported message received by %s", ctrl.pid)
	}
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

	serviceMsg := &ServiceMsg{Op: Operation{Op: UPDATE}, Service: service}

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

	serviceMsg := &ServiceMsg{Op: Op, Service: id}

	// We won't do anything specific to this endpoint. We will just ask the Service to recreate all the endpoints
	// belonging to this service if the service is actually being exposed.
	ctrl.pid.Tell(serviceMsg)

}

func (ctrl *controller) ingressCreateAndUpdate(obj interface{}) {
	ingress, ok := obj.(*V1Beta1api.Ingress)
	if !ok {
		log.Printf("Expected an object of type `*v1beta1.Ingress`, but got object of type: %s", reflect.TypeOf(obj))
		return
	}

	log.Printf("Received an object of type: %s, Obj: %v", reflect.TypeOf(obj), *ingress)

	namespace := ingress.GetNamespace()

	it, _ := iterable.New(ingress.Spec.Rules)

	// Process all the rules.
	observable.From(it).Subscribe(observer.Observer{
		// For every VHost that we get, register it with the load-balancer.
		NextHandler: func(item interface{}) {
			ingressRule := item.(*V1Beta1api.IngressRule)
			// Ask the controller to process this rule.
			ctrl.pid.Tell(&IngressMsg{
				Op:          Operation{Op: ADD},
				Namespace:   namespace,
				IngressRule: *ingressRule})
		},
	})
}

func (ctrl *controller) ingressDeleted(obj interface{}) {
	ingress, ok := obj.(*V1Beta1api.Ingress)
	if ok {
		log.Printf("Expected an object of type `*v1beta1.Ingress`, but got object of type: %s", reflect.TypeOf(obj))
	}
	log.Printf("Received an object of type: %s, Obj: %v", reflect.TypeOf(obj), *ingress)

	it, _ := iterable.New(ingress.Spec.Rules)

	// Process all the rules.
	observable.From(it).Subscribe(observer.Observer{
		// For every VHost that we get, register it with the load-balancer.
		NextHandler: func(item interface{}) {
			ingressRule := item.(*V1Beta1api.IngressRule)
			// Ask the controller to process this rule.
			ctrl.pid.Tell(&IngressMsg{
				Op:          Operation{Op: DEL},
				Namespace:   ingress.GetNamespace(),
				IngressRule: *ingressRule})
		},
	})
}

// Create a `VHost` based on an `IngressRule`.
func (ctrl *controller) _ingressCreateAndUpdate(namespace string, rule V1Beta1api.IngressRule) {
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

	return
}

// Delete a `Vhost` based on an IngressRule.
func (ctrl *controller) _ingressDeleted(namespace string, rule V1Beta1api.IngressRule) {
	// If a VHost already exists delete it since we will be re-creating it here.
	if _, ok := ctrl.vhosts[rule.Host]; ok {
		delete(ctrl.vhosts, rule.Host)
	}

	for _, path := range rule.HTTP.Paths {
		url := state.URL{Host: rule.Host, Path: path.Path}
		id := state.ServiceName{Namespace: namespace, Name: path.Backend.ServiceName}
		service, ok := ctrl.services[id.String()]
		// Store the association of the service with the URI
		if !ok {
			return
		}

		delete(service.URLs, url.String())
	}

	return
}

// Takes a `Service` and updates the endpoints of the service.
func (ctrl *controller) updateServiceEndpoints(id state.ServiceName) {
	service, ok := ctrl.services[id.String()]

	if !ok {
		// We don't have a VHost corresponding to this servcie so we don't need to do anything.
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

	return
}

func (ctrl *controller) deleteService(id state.ServiceName) {
	_, ok := ctrl.services[id.String()]

	if !ok {
		// We don't have a VHost corresponding to this servcie so we don't need to do anything.
		return
	}

	delete(ctrl.services, id.String())

	//TODO: Need to send the service update down to the load-balancer for the load-balacner to update its backend for a given VHOST.
}
