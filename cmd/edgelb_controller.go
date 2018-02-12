/*
Copyright 2016 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Note: the example only works with the code within the same release/branch.
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	// Edgelb-k8s
	"edgelb-k8s/pkg/ingress"
	"edgelb-k8s/pkg/lb/edgelb"

	// Actor
	"github.com/AsynkronIT/protoactor-go/actor"

	// k8s
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func internalClient() (clientset *kubernetes.Clientset) {
	// creates the in-cluster config
	log.Printf("Creating the config for connecting to the API server from within the cluster")
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	// creates the clientset
	clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	return
}

func externalClient() (clientset *kubernetes.Clientset) {
	var kubeconfig *string
	if home := homeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
		log.Printf("Loading kubeconfig: %v", *kubeconfig)
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	return
}

func main() {
	var clientset *kubernetes.Clientset
	internalPtr := flag.Bool("internal", false, "Internal to the k8s cluster")

	flag.Parse()

	if !*internalPtr {
		clientset = externalClient()
	} else {
		clientset = internalClient()
	}

	// Initialize Edge-LB.
	// NOTE: You need to expose the DC/OS credentials for `edge-lb` as a base64 secret at the path `/dcos/secrets/edge-lb-secret.json`
	edgelbBackend, err := edgelb.New("edgelb", "https://leader.mesos", "/dcos/secrets/edge-lb-secret.json")
	if err != nil {
		panic(err.Error())
	}

	log.Printf("Initialized the Edge-LB backend.")

	ctrl := ingress.NewController(clientset, actor.Spawn(actor.FromInstance(edgelbBackend)))
	log.Printf("Initialized the ingress controller.")

	err = ctrl.Start()

	log.Printf("Exiting the wait for K8s API server ....")
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
