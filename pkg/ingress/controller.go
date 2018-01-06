package ingress

import (
	"fmt"
	"log"
	"reflect"
	"time"

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

// Controller representation of a k8s service. This is primarily required to track the end-points.
type Service struct {
	Namespace string
	Name      string
	Endpoints []string
}

type controller struct {
	si               informers.SharedInformerFactory
	endpoints        v1.EndpointsLister           // All the endpoints that are availabe in a k8s cluster.
	ingressResources v1beta1.IngressLister        // Ingress resource that define the config for the controller.
	services         map[Service]map[VHost]string // Services for which the controller is asked to setup ingress.
	vhosts           map[string]VHost
}

type VHost struct {
	Host   string
	Routes []Route
}

type Route struct {
	Path    string
	Service ingress.Service
}

func NewController(clientset *kubernetes.Clientset) (ctrl Controller, err error) {
	resyncPeriod := 30 * time.Minute
	si := informers.NewSharedInformerFactory(clientset, resyncPeriod)

	ingressCtrl := controller{
		si:               si,
		endpoints:        si.Core().V1().Endpoints().Lister(),
		ingressResources: si.Extensions().V1beta1().Ingresses().Lister(),
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

			return ctrl.convertRuleToVHost(namespace, ingressRule)
		}).Subscribe(observer.Observer{
		// For every VHost that we get, register it with the load-balancer.
		NextHandler: func(item interface{}) {
			log.Printf("Need to register VHOST %v with load-balancer", item.(VHost))
		},
	})
}

func (ctrl *controller) ingressDeleted(obj interface{}) {
	ingress, ok := obj.(*V1Beta1api.Ingress)
	if ok {
		log.Printf("Expected an object of type `*v1beta1.Ingress`, but got object of type: %s", reflect.TypeOf(obj))
	}
	log.Printf("Received an object of type: %s, Obj: %v", reflect.TypeOf(obj), *ingress)
}

// Takes an `IngressRule`  and converts it to a corresponding `Route` object.
func (ctrl *controller) convertRuleToVHost(namespace string, rule *V1Beta1api.IngressRule) (vhost VHost) {
	vhost.host = rule.Host

	for _, path := range rule.HTTP.Paths {
		vhost.routes = append(vhost.routes, ctrl.convertPathToRoute(namespace, path))
	}

	return vhost
}

// Takes a `Path` and converts it to Route.
func (ctrl *controller) convertPathToRoute(namespace string, path V1Beta1api.HTTPIngressPath) (route Route) {
	route.path = path.Path
	route.Service = Service{namespace: namespace, name: path.Backend.ServiceName}

	// Register the service name with the controller.
	ctrl.services[route.Service] = true

	// Look at the service name, and get the corresponding endpoints for this service name.
	endpoints, err := ctrl.endpoints.Endpoints(namespace).Get(path.Backend.ServiceName)
	if err != nil {
		for _, endpoint := range endpoints.Subsets {
			for _, address := range endpoint.Addresses {
				for _, port := range endpoint.Ports {
					route.endpoints = append(route.endpoints, fmt.Sprintf("%s:%d", address.IP, port.Port))
				}
			}

			for _, address := range endpoint.NotReadyAddresses {
				for _, port := range endpoint.Ports {
					route.endpoints = append(route.endpoints, fmt.Sprintf("%s:%d", address.IP, port.Port))
				}
			}

		}
	} else {
		log.Printf("Unable to retrieve the endpoints:%s/%s, error:%s", namespace, path.Backend.ServiceName, err)
	}

	return
}
