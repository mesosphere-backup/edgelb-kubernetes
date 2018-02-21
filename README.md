# Introduction
`edgelb-k8s` is a kubernetes controller, meant to be run as a container either within, or outside, a kubernetes cluster. The controller talks to the Kubernetes API server to learn about ingress resources, services references by ingress resources and endpoints associated with those services. The controller uses the ingress resources and the associated information (services/endpionts) to than talk to `Edge-lb` and uses the Edge-lb APIs to program a load-balancing pool that provides ingress to a given Kubernetes cluster.

# Build
All the build pre-reqs are captured in Docker containers, and hence the only pre-req to run the build is `Docker`. The accompanying *Makefile* allows you to build the following targets:

## make edgelb_controller
Builds the *edgelb_controller* binary. This is a statically compiled `elf` GoLang binary. So it can technically be run on a linux distribution without any additional dependencies.

## make package
Creates a docker container `mesosphere/edgelb-k8s-controller`, that can be launched as a pod in Kubernetes, or a UCR/Docker container on DC/OS. By default the pod is setup to assume that it is running within a Kubernetes cluster, which would give it access to default service accounts setup by Kubernetes for all pods launched within the cluster.

## make vendor
Updates vendor dependencies for all golang packages.

# Architecture
The `edgelb-k8s` controller has two entities:
a) Ingress controller that talks to the Kubernetes API server and learns about ingress resources and all other referenced resources (services/endpoints). 
b) An edge-lb backend that talks to the Edge-LB API server to launch load-balancing pool for a given Kubernetes cluster.

To simplify concurrency and state management within the system we use a GoLang actor programming model. The package used is:

```bash
github.com/AsynkronIT/protoactor-go/actor
```

Using the above model both the ingress controller ad the edge-lb backend are actors within the system and use message passing to learn and configure pools.

# TODOs:
* Support TLS in ingress resources.
* Support API for accessing DC/OS secrets and storing ingress TLS in DC/OS secrets.
* Support ingress resource meta-data to allow pools to bind to different interfaces/ports.
* Support for Kubernetes `ConfigMap` resource in order to support user-defined configuration at startup.

# Open questions 
* Do we run the `edgelb-k8s` controller as Kubernetes pod or DC/OS pod?
* Who sets up DC/OS service account credential for the pod?
* How do we support multiple Kubernetes clusters? (Currently the pool names "k8s", may be we need to make it configurable.)
* How does the controller work in Open DC/OS, since Open DC/OS does not have service accounts? (Probably have the user configure the Kubernetes pool)