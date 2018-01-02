package ingress

import (
	"log"
	"reflect"
	"time"

	V1api "k8s.io/api/core/v1"
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
	pods             v1.PodLister
	services         v1.ServiceLister
	ingressResources v1beta1.IngressLister
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

	// Add watchers for pods
	si.Core().V1().Pods().Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    ingressCtrl.podCreated,
			DeleteFunc: ingressCtrl.podDeleted,
		},
	)

	// Add watchers for services
	si.Core().V1().Services().Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    ingressCtrl.serviceCreated,
			DeleteFunc: ingressCtrl.serviceDeleted,
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
	log.Printf("Received an object of type: %s", reflect.TypeOf(obj))
}

func (ctrl *controller) ingressDeleted(obj interface{}) {
	log.Printf("Received an object of type: %s", reflect.TypeOf(obj))
}
