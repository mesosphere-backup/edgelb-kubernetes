#docker images
GOIMAGE=golang:1.9-alpine
GLIDEIMAGE=instrumentisto/glide:0.13.1-go1.9

#docker build env
GOBUILDENV=-e GOPATH='/go' -e CGO_ENABLED=0 -e GOOS='linux' -e GOARCH=amd64

all: edgelb_controller

.PHONY: vendor package edgelb_controller

clean: edgelb_controller edgelb_client
	rm -rf $^

edgelb_controller:
	docker run --rm -t -v `pwd`:/go/src/edgelb-k8s\
			${GOBUILDENV}\
			-w='/go/src/edgelb-k8s'\
			${GOIMAGE}\
			go build -ldflags '-w -extldflags "-static"' cmd/edgelb_controller.go

edgelb_client:cmd/edgelb_client.go
	docker run --rm -t -v `pwd`:/go/src/edgelb-k8s\
			${GOBUILDENV}\
			-w='/go/src/edgelb-k8s'\
			${GOIMAGE}\
			go build -ldflags '-w -extldflags "-static"' cmd/edgelb_client.go 

vendor:
	docker run --rm -t -v `pwd`:/go/src/edgelb-k8s\
			-v ${HOME}/.ssh:/root/.ssh\
			-v ${HOME}/.git-credentials:/root/.git-credentials\
			-v ${HOME}/.gitconfig:/root/.gitconfig\
			-v ${HOME}/.glide:/root/.glide\
			${GOBUILDENV}\
			-w='/go/src/edgelb-k8s'\
			${GLIDEIMAGE}\
			update --strip-vendor

package:edgelb_controller
	docker build -t mesosphere/edgelb-k8s-controller ./
	docker push mesosphere/edgelb-k8s-controller
