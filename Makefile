GOPATH=`pwd`
GOBUILDENV="-e GOPATH='/go' -e CGO_ENABLED=0 -e GOOS='linux'"
all:edgelb_controller

.PHONY: vendor package


clean:
	rm -rf edgelb_controller

edgelb_controller:cmd/edgelb_controller.go
	docker run --rm -t -v `pwd`:/go/src/edgelb-k8s\
			${GOBUILDENV}\
			-w='/go/src/edgelb-k8s'\
			golang:1.9-alpine\
			go build -ldflags '-w -extldflags "-static"' $^

edgelb_client:cmd/edgelb_client.go
	docker run --rm -t -v `pwd`:/go/src/edgelb-k8s\
			${GOBUILDENV}\
			-w='/go/src/edgelb-k8s'\
			golang:1.9-alpine\
			go build -ldflags '-w -extldflags "-static"' $^

vendor:
	docker run --rm -t -v `pwd`:/go/src/edgelb-k8s\
			-v ${HOME}/.ssh:/root/.ssh\
			-v ${HOME}/.git-credentials:/root/.git-credentials\
			-v ${HOME}/.gitconfig:/root/.gitconfig\
			-v ${HOME}/.glide:/root/.glide\
			${GOBUILDENV}\
			-w='/go/src/edgelb-k8s'\
			instrumentisto/glide:0.13.1-go1.9\
			update --strip-vendor

package:edgelb_controller edgelb_client
	docker build -t mesosphere/edgelb-k8s-controller ./
	docker push mesosphere/edgelb-k8s-controller
