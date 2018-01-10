all:edgelb_controller

clean:
	rm -rf edgelb_controller

edgelb_controller:cmd/edgelb_controller.go
	go build cmd/edgelb_controller.go

vendor:
	glide update --strip-vendor
