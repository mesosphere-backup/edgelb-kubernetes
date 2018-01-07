package ingress

import (
	"fmt"
	"log"
	"reflect"
	"time"

	// Ingress controller
	"github.com/asridharan/edgelb-k8s/pkg/state"

	// RxGo
	"github.com/reactivex/rxgo/errors"
	"github.com/reactivex/rxgo/iterable"
	"github.com/reactivex/rxgo/observable"
	"github.com/reactivex/rxgo/observer"

	// K8s
	V1Beta1api "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/listers/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
)

type Controller interface {
	Start()
}

type controller struct {
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
			AddFunc:    ingressCtrl.endpointCreated,
			UpdateFunc: ingressCtrl.endpointUpdated,
			DeleteFunc: ingressCtrl.endpointDeleted,
		},
	)

	// Add watchers for ingress
	si.Extensions().V1beta1().Ingresses().Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    ingressCtrl.ingressCreated,
			DeleteFunc: ingressCtrl.ingressDeleted,
		},
	)

	return
}

func (ctrl *controller) Start() {
	ctrl.si.Start(wait.NeverStop)
	<-wait.NeverStop
}

func (ctrl *controller) endpointCreated(obj interface{}) {
}

func (ctrl *controller) endpointDeleted(obj interface{}) {
}

func (ctrl *controller) endpointUpdated(old interface{}, new interface{}) {
}

func (ctrl *controller) ingressCreated(obj interface{}) {
	ingress, ok := obj.(*V1Beta1api.Ingress)
	if ok {
		log.Printf("Expected an object of type `*v1beta1.Ingress`, but got object of type: %s", reflect.TypeOf(obj))
		return
	}

	log.Printf("Received an object of type: %s, Obj: %v", reflect.TypeOf(obj), *ingress)

	namespace := ingress.Namespace

	it, _ := iterable.New(ingress.Spec.Rules)

	// Process all the rules.
	observable.From(it).Map(
		// Convert an IngressRule to a VHost object.
		func(obj interface{}) interface{} {
			ingressRule, ok := obj.(*V1Beta1api.IngressRule)
			if !ok {
				return errors.New(errors.ObservableError, fmt.Sprintf("Expected an object of type `*v1beta1.Ingress`, but got object of type: %s", reflect.TypeOf(obj)))

			}

			return ctrl.newVHost(namespace, ingressRule)
		}).Subscribe(observer.Observer{
		// For every VHost that we get, register it with the load-balancer.
		NextHandler: func(item interface{}) {
			log.Printf("Need to register VHOST %v with load-balancer", item.(*state.VHost))
		},
	})
}

func (ctrl *controller) ingressUpdated(old interface{}, new interface{}) {
}

func (ctrl *controller) ingressDeleted(obj interface{}) {
	ingress, ok := obj.(*V1Beta1api.Ingress)
	if ok {
		log.Printf("Expected an object of type `*v1beta1.Ingress`, but got object of type: %s", reflect.TypeOf(obj))
	}
	log.Printf("Received an object of type: %s, Obj: %v", reflect.TypeOf(obj), *ingress)
}

// Takes an `IngressRule`  and converts it to a corresponding `Route` object.
func (ctrl *controller) newVHost(namespace string, rule *V1Beta1api.IngressRule) (vhost *state.VHost) {
	// If a VHost already exists delete it since we will be re-creating it here.
	if _, ok := ctrl.vhosts[rule.Host]; ok {
		delete(ctrl.vhosts, rule.Host)
	}
	vhost = &state.VHost{Host: rule.Host}

	for _, path := range rule.HTTP.Paths {
		url := state.URL{Host: rule.Host, Path: path.Path}
		id := state.ServiceName{Namespace: namespace, Name: path.Backend.ServiceName}
		var service *state.Service
		// Store the association of the service with the URI
		if _, ok := ctrl.services[service.String()]; !ok {
			ctrl.services[id.String()] = &state.Service{ServiceName: id}
		}
		service = ctrl.services[id.String()]
		service.URLs = append(service.URLs, url)

		ctrl.updateServiceEndpoints(service)

		// Append the route to the VHost.
		vhost.Routes = append(vhost.Routes, state.Route{Path: url.Path, ServiceName: id})
	}

	ctrl.vhosts[vhost.Host] = vhost

	return vhost
}

// Takes a `Service` and Updates the endpoints of the service.
func (ctrl *controller) updateServiceEndpoints(service *state.Service) {
	// Since we might actually be adding existing endpoints to the service,
	// remove the existing endpoints from the service before adding new ones.
	service.Endpoints = nil

	// Look at the service name, and get the corresponding endpoints for this service name.
	endpoints, err := ctrl.endpoints.Endpoints(service.Namespace).Get(service.Name)
	if err != nil {
		for _, endpoint := range endpoints.Subsets {
			for _, address := range endpoint.Addresses {
				for _, port := range endpoint.Ports {
					service.Endpoints = append(service.Endpoints, fmt.Sprintf("%s:%d", address.IP, port.Port))
				}
			}

			for _, address := range endpoint.NotReadyAddresses {
				for _, port := range endpoint.Ports {
					service.Endpoints = append(service.Endpoints, fmt.Sprintf("%s:%d", address.IP, port.Port))
				}
			}

		}
	} else {
		log.Printf("Unable to retrieve the endpoints for service:%s, error:%s", *service, err)
	}

	return
}
