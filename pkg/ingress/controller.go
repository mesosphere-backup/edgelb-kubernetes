package ingress

import (
	"log"
	"reflect"
	"time"

	// RxGo
	"github.com/reactivex/rxgo/errors"
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

type Controller interface {
	Start()
}

// Controller representation of a k8s service. This is primarily required to track the end-points.
type service struct {
	namespace string
	name      string
}

type controller struct {
	si               informers.SharedInformerFactory
	endpoints        v1.EndpointLister
	ingressResources v1beta1.IngressLister
	ingressServices  map[service]bool
}

type VHost struct {
	host   string
	routes []Route
}

type Route struct {
	path     string
	backends []string
}

func NewController(clientset *kubernetes.Clientset) (ctrl Controller, err error) {
	resyncPeriod := 30 * time.Minute
	si := informers.NewSharedInformerFactory(clientset, resyncPeriod)

	ingressCtrl := controller{
		si:               si,
		pods:             si.Core().V1().Pods().Lister(),
		services:         si.Core().V1().Services().Lister(),
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

func (ctrl *controller) podCreated(obj interface{}) {
	pod, ok := obj.(*V1api.Pod)
	if !ok {
		log.Printf("Expected an object of type `*v1.Pod`, but got object of type: %s", reflect.TypeOf(obj))
	}

	log.Printf("Received an object of type: %s, Obj: %v", reflect.TypeOf(obj), *pod)
}

func (ctrl *controller) podDeleted(obj interface{}) {
	pod, ok := obj.(*V1api.Pod)
	if ok {
		log.Printf("Expected an object of type `*v1.Pod`, but got object of type: %s", reflect.TypeOf(obj))
	}

	log.Printf("Received an object of type: %s, Obj: %v", reflect.TypeOf(obj), *pod)
}

func (ctrl *controller) serviceCreated(obj interface{}) {
	service, ok := obj.(*V1api.Service)
	if !ok {
		log.Printf("Expected an object of type `*v1.Service`, but got object of type: %s", reflect.TypeOf(obj))
	}

	log.Printf("Received an object of type: %s, Obj: %v", reflect.TypeOf(obj), *service)
}

func (ctrl *controller) serviceDeleted(obj interface{}) {
	service, ok := obj.(*V1api.Service)
	if ok {
		log.Printf("Expected an object of type `*v1.Service`, but got object of type: %s", reflect.TypeOf(obj))
	}

	log.Printf("Received an object of type: %s, Obj: %v", reflect.TypeOf(obj), *service)
}

func (ctrl *controller) ingressCreated(obj interface{}) {
	ingress, ok := obj.(*V1Beta1api.Ingress)
	if ok {
		log.Printf("Expected an object of type `*v1beta1.Ingress`, but got object of type: %s", reflect.TypeOf(obj))
	}

	log.Printf("Received an object of type: %s, Obj: %v", reflect.TypeOf(obj), *ingress)

	namespace := ingress.Namespace

	it := iterable.New(ingress.Spec.Rules)

	// Process all the rules.
	observable.From(it).Map(
		// Convert an IngressRule to a VHost object.
		func(obj interface{}) interface{} {
			ingressRule, ok := obj.(*V1Beta1api.IngressRule)
			if !ok {
				return errors.New(errors.ObservableError, "Expected an object of type `*v1beta1.Ingress`, but got object of type: %s", reflect.TypeOf(obj))

			}

			return ctrl.convertRuleToVHost(namespace, ingressRule)
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
func (ctrl *controller) convertRuleToVHost(namespace string, rule V1Beta1api.IngressRule) (vhost VHost) {
	vhost.host = rule.Host

	for path := range rule.HTTP.Paths {
		append(vhost.routes, ctrl.convertPathToRoute(namespace, path))
	}

	return vhost
}

// Takes a `Path` and converts it to Route.
func (ctrl *controller) convertPathToRoute(namespace string, path V1Beta1api.HTTPIngressPath) (route Route) {
	route.path = path.Path

	// Look at the service name, and get the corresponding backends for this service name.
	service, err := ctrl.services.Services(namespace).Get(path.ServiceName)
	if err != nil {
		// Create a selector for this service and use it to find pods.
		selector := labels.SelectorFromSet(service.Spec.Selector)
		// Get a list of pods based on this selector.
		pods, err := ctrl.pods.Pods(namespace).List()

		if err != nil {
			for pod := range pods {
				append(route.backend, pod.Status.PodIP)
			}
		} else {
			log.Printf(" Unable to retrieve pods for the given service:%s/%s, error:%s", namespace, path.ServiceName, err)
		}
	} else {
		log.Printf("Unable to retrieve the Service:%s/%s, error:%s", namespace, path.ServiceName, err)
	}
}
