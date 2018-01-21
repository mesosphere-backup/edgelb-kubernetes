############## Setup
DOCKER_REPO ?= mesosphere
APISERVER_DOCKER_REPO ?= $(DOCKER_REPO)
APISERVER_DOCKER_NAME ?= dapis
APISERVER_DOCKER ?= $(APISERVER_DOCKER_REPO)/$(APISERVER_DOCKER_NAME)
DOCKER_VERSION ?= v0.0.0
APISERVER_VERSION ?= $(DOCKER_VERSION)
REPO ?= github.com/mesosphere/dcos-edge-lb
GOREPO ?= $(GOPATH)/src/$(REPO)
CGO_ENABLED ?= 0
GOOS ?= linux
GOARCH ?= amd64
GOBUILDFLAGS ?= CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH)
API_BIN ?= apiserver
SETUP_BIN ?= setup
GIT_TAG := $(shell git describe --tags)
GO_LDFLAGS ?= -X github.com/mesosphere/dcos-edge-lb/apiserver/version.edgelbVersionString=$(GIT_TAG)

CONTAINERFILES ?= containerfiles
TASK_BIN_DIR ?= $(CONTAINERFILES)/apiserver/bin
LISTENER_BIN ?= mesos-listener
DCOSTEMPLATE_BIN ?= dcos-template

BIN_SUFFIX ?= $(GOOS)_$(GOARCH)

############## Begin Targets
.PHONY: clean deps vendor-save dockerize push spec
all: spec dockerize

clean:
	$(MAKE) -C spec clean
	rm -f $(API_BIN)_$(BIN_SUFFIX)
	rm -f $(SETUP_BIN)_$(BIN_SUFFIX)
	rm -f $(LISTENER_BIN)_$(BIN_SUFFIX)
	rm -f $(TASK_BIN_DIR)/$(API_BIN)
	rm -f $(TASK_BIN_DIR)/$(LISTENER_BIN)
	rm -f $(DCOSTEMPLATE_BIN)_$(BIN_SUFFIX)
	rm -f $(TASK_BIN_DIR)/$(DCOSTEMPLATE_BIN)
	rm -f .dockerized
	rm -f .pushed

# make spec may exit if there is nothing to do, ie content already generated
spec: deps
	$(MAKE) -s -C spec

$(API_BIN)_$(BIN_SUFFIX):
	$(GOBUILDFLAGS) go build -ldflags "$(GO_LDFLAGS)" -o $(API_BIN)_$(BIN_SUFFIX) -a -installsuffix cgo ./cmd/dcos-edge-lb-server/
	cp $(API_BIN)_$(BIN_SUFFIX) $(TASK_BIN_DIR)/$(API_BIN)

$(SETUP_BIN)_$(BIN_SUFFIX):
	$(GOBUILDFLAGS) go build -o $(SETUP_BIN)_$(BIN_SUFFIX) ./setup/
	cp $(SETUP_BIN)_$(BIN_SUFFIX) $(TASK_BIN_DIR)/$(SETUP_BIN)

$(LISTENER_BIN)_$(BIN_SUFFIX):
	$(GOBUILDFLAGS) go build -o $(LISTENER_BIN)_$(BIN_SUFFIX) ./mesos-listener/
	cp $(LISTENER_BIN)_$(BIN_SUFFIX) $(TASK_BIN_DIR)/$(LISTENER_BIN)

$(DCOSTEMPLATE_BIN)_$(BIN_SUFFIX):
	$(GOBUILDFLAGS) go build -o $(DCOSTEMPLATE_BIN)_$(BIN_SUFFIX) ./dcos-template
	cp $(DCOSTEMPLATE_BIN)_$(BIN_SUFFIX) $(TASK_BIN_DIR)/$(DCOSTEMPLATE_BIN)

.dockerized: $(API_BIN)_$(BIN_SUFFIX) $(LISTENER_BIN)_$(BIN_SUFFIX) $(DCOSTEMPLATE_BIN)_$(BIN_SUFFIX)
	docker build -t $(APISERVER_DOCKER) .
	docker tag $(APISERVER_DOCKER):latest $(APISERVER_DOCKER):$(APISERVER_VERSION)
	touch .dockerized
	rm -f .pushed

dockerize: .dockerized

.pushed:
	docker push $(APISERVER_DOCKER):latest
	docker push $(APISERVER_DOCKER):$(APISERVER_VERSION)
	touch .pushed

push: .pushed

# TODO: determine what environment this is supposed to work w/i
# `make test` in this subdir has many errors, ie:
# ... error=NewManager zookeeper failed to create node zk-1.zk:2181,zk-2.zk:2181,zk-3.zk:2181,zk-4.zk:2181,zk-5.zk:2181: error checking existence zk data node: zk: could not connect to a server ...
test:
	docker rm -vf $(APISERVER_DOCKER_NAME) || true
	docker run -d --name $(APISERVER_DOCKER_NAME) --privileged -e ELB_FILE_TEST="`echo 'my\ncool\nfile'`" -e PORTS=80,443 -e ELB_POOL_NAME=testpool -e ELB_ZKPATH="/edgelb" -p 52978:52978 -it $(APISERVER_DOCKER):$(APISERVER_VERSION) /dcosfiles/apiserver/bin/run
	# docker exec $(APISERVER_DOCKER_NAME) bash -c "until cp /etc/resolv.conf /etc/resolv.conf.bak; do sleep 1; done"
	# docker exec $(APISERVER_DOCKER_NAME) bash -c "until echo 'nameserver 198.51.100.1' > /etc/resolv.conf && touch /etc/resolv.conf; do sleep 1; done"
	docker logs -f $(APISERVER_DOCKER_NAME)

############## Includes
# Include all .mk files so you can have your own local configurations
include $(wildcard ../*.mk)
