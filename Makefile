all:edgelb_controller

.PHONY: vendor package

clean:
	rm -rf edgelb_controller

edgelb_controller:cmd/edgelb_controller.go
	docker run --rm -t -v `pwd`:/go/src/edgelb-k8s\
			-e GOPATH='/go'\
			-e CGO_ENABLED=0\
			-e GOOS='linux'\
			-w='/go/src/edgelb-k8s'\
			golang:1.9-alpine\
			go build -ldflags '-w -extldflags "-static"' cmd/edgelb_controller.go

vendor:
	glide update --strip-vendor

package:edgelb_controller
	docker build -t mesosphere/edgelb-k8s-controller ./
	docker push mesosphere/edgelb-k8s-controller
