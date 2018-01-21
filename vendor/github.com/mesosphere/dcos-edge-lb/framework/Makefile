############## Setup
DOCKER_REPO ?= mesosphere
DOCKER_VERSION ?= v0.0.0
LBTASK_DOCKER_REPO ?= $(DOCKER_REPO)
LBTASK_DOCKER_NAME ?= dhap
LBTASK_DOCKER ?= $(LBTASK_DOCKER_REPO)/$(LBTASK_DOCKER_NAME)
LBTASK_VERSION ?= $(DOCKER_VERSION)
APISERVER_DOCKER_REPO ?= $(DOCKER_REPO)
APISERVER_DOCKER_NAME ?= dapis
APISERVER_DOCKER ?= $(APISERVER_DOCKER_REPO)/$(APISERVER_DOCKER_NAME)
APISERVER_VERSION ?= $(DOCKER_VERSION)
S3_BUCKET ?= infinity-artifacts
S3_DIR_PATH ?= autodelete7d
POOL_S3_URL ?= s3://$(S3_BUCKET)/$(S3_DIR_PATH)/edgelb-pool
POOL_S3_DIR ?= https://$(S3_BUCKET).s3.amazonaws.com/$(S3_DIR_PATH)/edgelb-pool
# POOL_UNIVERSE ?= $(POOL_S3_DIR)/stub-universe-edgelb-pool.zip
POOL_UNIVERSE ?= $(POOL_S3_DIR)/stub-universe-edgelb-pool.json
API_S3_URL ?= s3://$(S3_BUCKET)/$(S3_DIR_PATH)/edgelb
API_S3_DIR ?= https://$(S3_BUCKET).s3.amazonaws.com/$(S3_DIR_PATH)/edgelb
# API_UNIVERSE ?= $(API_S3_DIR)/stub-universe-edgelb.zip
API_UNIVERSE ?= $(API_S3_DIR)/stub-universe-edgelb.json
GO_BINDATA_VERSION ?= v3.0.5
BINDATA_FILES = $(CURDIR)/edgelb/cli/dcos-edgelb/commands/reference_bindata.go
GIT_TAG ?= $(shell git describe --tags)

export GO_LDFLAGS ?= -X github.com/mesosphere/dcos-edge-lb/apiserver/version.edgelbVersionString=$(GIT_TAG)

############## Begin Targets
.PHONY: clean compile build push
all: build

clean:
	rm -rf edgelb-pool/build
	rm -rf edgelb-pool/cli/dcos-edgelb-pool/native-*
	rm -rf edgelb-pool/cli/dcos-edgelb-pool/dcos-*
	rm -rf edgelb-pool/cli/dcos-edgelb-pool/bin_wrapper-*
	rm -rf edgelb/build
	rm -rf edgelb/cli/dcos-edgelb/native-*
	rm -rf edgelb/cli/dcos-edgelb/dcos-*
	rm -rf edgelb/cli/dcos-edgelb/bin_wrapper-*
	rm -f .edgelb-pushed
	rm -f .edgelb-pool-pushed
	rm -f .edgelb-built
	rm -f .edgelb-pool-built
	rm -f $(BINDATA_FILES)

maybe-install-go-bindata:
	# I tried "gopkg.in/jteeuwen/go-bindata.v3" but "gopkg.in" only operates on
	# major versions which is insufficient for this use case. This is ok for
	# vendoring as that saves the actual source, but doesn't guarantee reliably
	# installing a binary.
	type go-bindata || \
		( \
			go get -d github.com/jteeuwen/go-bindata  && \
			cd $(GOPATH)/src/github.com/jteeuwen/go-bindata && \
			git checkout $(GO_BINDATA_VERSION) && \
			go install ./... && \
			type go-bindata \
		)

go-generate: maybe-install-go-bindata
	go generate ./...

.edgelb-built: go-generate
	export S3_URL=$(API_S3_URL) && export ARTIFACT_DIR=$(API_S3_DIR) && \
		export APISERVER_DOCKER=$(APISERVER_DOCKER) && export APISERVER_VERSION=$(APISERVER_VERSION) && \
		cd edgelb/ && ./build.sh
	touch .edgelb-built

.edgelb-pushed:
	export S3_URL=$(API_S3_URL) && export ARTIFACT_DIR=$(API_S3_DIR) && \
		export APISERVER_DOCKER=$(APISERVER_DOCKER) && export APISERVER_VERSION=$(APISERVER_VERSION) && \
		cd edgelb/ && ./build.sh aws
	touch .edgelb-pushed

.edgelb-pool-built:
	export S3_URL=$(POOL_S3_URL) && export ARTIFACT_DIR=$(POOL_S3_DIR) && \
		export LBTASK_DOCKER=$(LBTASK_DOCKER) && export LBTASK_VERSION=$(LBTASK_VERSION) && \
		cd edgelb-pool/ && ./build.sh
	touch .edgelb-pool-built

.edgelb-pool-pushed:
	export S3_URL=$(POOL_S3_URL) && export ARTIFACT_DIR=$(POOL_S3_DIR) && \
		export LBTASK_DOCKER=$(LBTASK_DOCKER) && export LBTASK_VERSION=$(LBTASK_VERSION) && \
		cd edgelb-pool/ && ./build.sh aws
	touch .edgelb-pool-pushed

build: $(BUILD_DIR) .edgelb-built .edgelb-pool-built

push: .edgelb-pushed .edgelb-pool-pushed

kill-repos:
	for repo in edgelb edgelb-pool; do \
		dcos package describe $$repo >/dev/null 2>&1 && \
			dcos package repo remove $${repo}-aws || \
			echo "$$repo is already removed"; \
	done

add-repos: kill-repos
	dcos package repo add --index=0 edgelb-aws $(API_UNIVERSE)
	dcos package repo add --index=0 edgelb-pool-aws $(POOL_UNIVERSE)

############## Includes
# Include all .mk files so you can have your own local configurations
include $(wildcard ../*.mk)
