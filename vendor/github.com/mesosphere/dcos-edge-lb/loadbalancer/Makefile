############## Setup
DOCKER_REPO ?= mesosphere
DOCKER_VERSION ?= v0.0.0
LBTASK_DOCKER_REPO ?= $(DOCKER_REPO)
LBTASK_DOCKER_NAME ?= dhap
LBTASK_DOCKER ?= $(LBTASK_DOCKER_REPO)/$(LBTASK_DOCKER_NAME)
LBTASK_VERSION ?= $(DOCKER_VERSION)
REPO ?= github.com/mesosphere/dcos-edge-lb
GOREPO ?= $(GOPATH)/src/$(REPO)
CONTAINERFILES ?= containerfiles
TASK_BIN_DIR ?= $(CONTAINERFILES)/haproxy/bin
LBMGR_BIN ?= lbmgr
CGO_ENABLED ?= 0
GOOS ?= linux
GOARCH ?= amd64
GOBUILDFLAGS ?= CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH)
BIN_SUFFIX ?= $(GOOS)_$(GOARCH)

############## Begin Targets
.PHONY: clean vendor-save dockerize push test
all: dockerize

clean:
	rm -f $(LBMGR_BIN)_$(BIN_SUFFIX)
	rm -f $(TASK_BIN_DIR)/$(LBMGR_BIN)
	rm -f .dockerized
	rm -f .pushed

$(LBMGR_BIN)_$(BIN_SUFFIX):
	$(GOBUILDFLAGS) go build -o $(LBMGR_BIN)_$(BIN_SUFFIX) ./lbmgr/
	cp $(LBMGR_BIN)_$(BIN_SUFFIX) $(TASK_BIN_DIR)/$(LBMGR_BIN)

.dockerized: $(LBMGR_BIN)_$(BIN_SUFFIX)
	docker build -t $(LBTASK_DOCKER) . && \
	docker tag $(LBTASK_DOCKER):latest $(LBTASK_DOCKER):$(LBTASK_VERSION)
	touch .dockerized
	rm -f .pushed

dockerize: .dockerized

.pushed:
	docker push $(LBTASK_DOCKER):latest
	docker push $(LBTASK_DOCKER):$(LBTASK_VERSION)
	touch .pushed

push: .pushed

test:
	docker rm -vf $(LBTASK_DOCKER_NAME) || true
	docker run -d --name $(LBTASK_DOCKER_NAME) --privileged -e ELB_FILE_TEST="`echo 'my\ncool\nfile'`" -e PORTS=80,443 -e ELB_POOL_NAME=testpool -e ELB_ZKPATH="/edgelb" -p 80:80 -p 443:443 -p 9090:9090 -it $(LBTASK_DOCKER):$(LBTASK_VERSION) /dcosfiles/haproxy/bin/run
	docker exec $(LBTASK_DOCKER_NAME) bash -c "until cp /etc/resolv.conf /etc/resolv.conf.bak; do sleep 1; done"
	docker exec $(LBTASK_DOCKER_NAME) bash -c "until echo 'nameserver 198.51.100.1' > /etc/resolv.conf && touch /etc/resolv.conf; do sleep 1; done"
	docker logs -f $(LBTASK_DOCKER_NAME)

############## Includes
# Include all .mk files so you can have your own local configurations
include $(wildcard ../*.mk)
