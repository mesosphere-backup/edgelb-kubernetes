export S3_BUCKET ?= infinity-artifacts
export DOCKER_REPO ?= mesosphere
export DOCKER_VERSION ?= v0.0.0
export LBTASK_DOCKER_REPO ?= $(DOCKER_REPO)
export LBTASK_DOCKER_NAME ?= dhap
export LBTASK_DOCKER ?= $(LBTASK_DOCKER_REPO)/$(LBTASK_DOCKER_NAME)
export LBTASK_VERSION ?= $(DOCKER_VERSION)
export APISERVER_DOCKER_REPO ?= $(DOCKER_REPO)
export APISERVER_DOCKER_NAME ?= dapis
export APISERVER_DOCKER ?= $(APISERVER_DOCKER_REPO)/$(APISERVER_DOCKER_NAME)
export APISERVER_VERSION ?= $(DOCKER_VERSION)
export DCOS_USER ?= core
GIT_TAG := $(shell git describe --tags)
export GO_LDFLAGS ?= -X github.com/mesosphere/dcos-edge-lb/apiserver/version.edgelbVersionString=$(GIT_TAG)

INTEGRATION_SCRIPT = scripts/ci-integration.sh

SSH_DIR := ~/.ssh
AWS_DIR := ~/.aws
SSH_GIT_KEY ?= id_rsa
# Docker credentials
DOCKER_USERNAME ?= nobody
DOCKER_PASSWORD ?= insecure
CONTAINER_RUN_COMMAND ?= ./ci-build
CONTAINER_MAKE_TARGET ?= push
CLUSTER_URL ?= unset
CLUSTER_USERNAME ?= nobody
CLUSTER_PASSWORD ?= insecure

# XXX If you ever run into package weirdness with `go install` or `go build`,
# it is likely some package caching issue (?), all I know is that adding the
# `-a` flag which forces rebuild of everything will generally fix it, but it
# makes the build super slow. Perhaps we can also investigate if a `go clean`
# can help.

GOPKG_SRCPATH := $(shell pwd | sed -e "s|${GOPATH}/src/||")
GOPKG_COMMAND = go list ./... | \
	grep -v '^github.com/mesosphere/dcos-edge-lb/vendor' | \
	grep -v '^github.com/mesosphere/dcos-edge-lb/framework/edgelb/vendor' | \
	grep -v '^github.com/mesosphere/dcos-edge-lb/framework/edgelb-pool/vendor'
GOPKG_COMMAND_NOAUTO = ${GOPKG_COMMAND} | \
	grep -v '^github.com/mesosphere/dcos-edge-lb/apiserver/client' | \
	grep -v '^github.com/mesosphere/dcos-edge-lb/apiserver/models' | \
	grep -v '^github.com/mesosphere/dcos-edge-lb/apiserver/restapi/operations' | \
	grep -v '^github.com/mesosphere/dcos-edge-lb/go-mesos-operator/include'

GOPKGS := $(shell ${GOPKG_COMMAND})
GOPKGS_SHORT_NOAUTO := $(shell ${GOPKG_COMMAND_NOAUTO} | sed -e 's|${GOPKG_SRCPATH}/||')

POOL_COUNT ?= 1
OSS = "false"

.PHONY: distclean clean oss-repo-exists deps compile push add-repos install test go-mesos-operator
all: compile

clean:
	find . -name __pycache__ | xargs rm -rf
	$(MAKE) -C apiserver clean
	$(MAKE) -C framework clean
	$(MAKE) -C loadbalancer clean

container-clean:
	for image in `docker image ls |awk -v user=${DOCKER_REPO} -v image=dcos-edge-lb-builder '//{ if($$1==user && $$2==image){print $$3} }'`; do \
		docker image rm -f $$image; \
	done

container-build: clean
	docker build . -t ${DOCKER_REPO}:dcos-edge-lb-builder

container-run:
	case x$(CONTAINER_RUN_COMMAND) in \
		x*sh) CONTAINER_RUN_FLAGS=-it;; \
	esac && \
	docker run -v /var/run/docker.sock:/var/run/docker.sock \
		$$CONTAINER_RUN_FLAGS \
		-v ${SSH_DIR}:/root/.ssh \
		-v ${AWS_DIR}:/root/.aws \
		-e ssh_git_key=${SSH_GIT_KEY} \
		-e DOCKER_USERNAME=${DOCKER_USERNAME} \
		-e DOCKER_PASSWORD=${DOCKER_PASSWORD} \
		-e MAKE_TARGET=${CONTAINER_MAKE_TARGET} \
		-e CLUSTER_URL=${CLUSTER_URL} \
		-e CLUSTER_USERNAME=${CLUSTER_USERNAME} \
		-e CLUSTER_PASSWORD=${CLUSTER_PASSWORD} \
		${DOCKER_REPO}:dcos-edge-lb-builder \
		${CONTAINER_RUN_COMMAND}

compile:
	$(MAKE) -C loadbalancer
	$(MAKE) -C apiserver
	$(MAKE) -C framework

push:
	$(MAKE) -C loadbalancer push
	$(MAKE) -C apiserver push
	$(MAKE) -C framework push

go-mesos-operator:
	$(MAKE) -C go-mesos-operator clean all

reinstall-apiserver:
	rm -f apiserver/*_linux_amd64
	rm -f apiserver/.dockerized
	$(MAKE) reinstall

reinstall-framework:
	rm -f framework/.edgelb-pushed
	rm -f framework/.edgelb-pool-pushed
	$(MAKE) reinstall

reinstall-framework-edgelb:
	rm -f framework/.edgelb-pushed
	$(MAKE) reinstall

reinstall-framework-edgelb-pool:
	rm -f framework/.edgelb-pool-pushed
	$(MAKE) reinstall

reinstall: uninstall install

bootstrap-integration:
	$(INTEGRATION_SCRIPT) bootstrap

lint-integration:
	$(INTEGRATION_SCRIPT) run_lint

debug-run-integration:
	$(eval export CLUSTER_URL ?= uninitialized)
	$(INTEGRATION_SCRIPT) debug_run_integration

# SINGLE_TEST should be set to something like "test_basic.py::test_ping"
debug-run-integration-single:
	$(eval export CLUSTER_URL ?= uninitialized)
	$(eval export SINGLE_TEST ?= uninitialized)
	$(INTEGRATION_SCRIPT) run_single_test "$(SINGLE_TEST)"

run-integration:
	# This is only for CI as it launches a new cluster
	$(INTEGRATION_SCRIPT) run_integration

delete-cluster-integration:
	$(INTEGRATION_SCRIPT) delete_cluster

maybe-install-dcos-cli:
	type dcos || \
		( \
			./scripts/dcos-cli-install.sh && \
			type dcos \
		)

install: uninstall
	$(MAKE) -C framework add-repos
	if [ ${OSS} = "true" ]; then \
		./scripts/install.sh; \
	else \
		./scripts/install-enterprise.sh; \
	fi

uninstall: maybe-install-dcos-cli
	./scripts/delete-pools.sh
	dcos task |grep 'api[.].*edgelb' >/dev/null 2>&1 && \
		dcos package uninstall edgelb --yes || \
		echo "edgelb is already uninstalled"
	for edge_group in dcos-edgelb/pools dcos-edgelb; do \
		pool_pods=`dcos marathon group show $$edge_group 2>&1 |jq '.["pods"] |length' 2>/dev/null`; \
			ec=$$?; \
			[ $$ec -eq 0 ] && [ 0 -le 0$$pool_pods ] && \
			dcos marathon group remove $$edge_group || \
			echo "$$edge_group is already removed"; \
	done
	$(MAKE) -C framework kill-repos
	for test_app in /bridge-nginx /host-httpd; do \
		dcos marathon app show $$test_app >/dev/null 2>&1 && \
			dcos marathon app remove $$test_app || \
			echo "$$test_app is already removed"; \
	done
	# TODO: leaving security bits installed. errors from their presence isn't choking up subsequent
	# `make install`, but this is not as clean as other items which are uninstalled.

test:
	# Handy flag is to set '-v'
	go test -ldflags "${GO_LDFLAGS}" ${TESTFLAGS} -race -timeout=10m ${GOPKGS}

go-install:
	go install -ldflags "${GO_LDFLAGS}" ./...

lint: go-install
	# go-install is required for the gotype linter to pass

	# XXX tests to investigate:
	# --exclude='TLS InsecureSkipVerify set true.,HIGH,HIGH'
	# --exclude='Subprocess launching with variable.,HIGH,HIGH'

	gometalinter \
		--vendor --concurrency=2 --cyclo-over=12 --tests \
		--enable='gofmt' \
		--enable='misspell' \
		--exclude='TLS InsecureSkipVerify may be true.' \
		--exclude='TLS InsecureSkipVerify set true.,HIGH,HIGH' \
		--exclude='Subprocess launching with variable.,HIGH,HIGH' \
		--exclude="apiserver/cmd/dcos-edge-lb-server/main.go" \
		--exclude="apiserver/restapi/server.go" \
		--exclude="apiserver/restapi/embedded_spec.go" \
		--exclude="apiserver/restapi/doc.go" \
		--exclude="apiserver/dcos-template/*" \
		--exclude="apiserver/models/v1_check.go:.* Potential hardcoded credentials,HIGH,LOW" \
		--exclude="apiserver/models/v2_check.go:.* Potential hardcoded credentials,HIGH,LOW" \
		--exclude="apiserver/models/common_check.go:.* Potential hardcoded credentials,HIGH,LOW" \
		--exclude="framework/edgelb/cli/dcos-edgelb/commands/reference_bindata.go" \
		--deadline=20m ${GOPKGS_SHORT_NOAUTO}

# test-http and test-https are mutually exclusive since they share the same marathon app name
test-http: maybe-install-dcos-cli
	until dcos edgelb ping; do sleep 1; done
	dcos marathon app add examples/apps/host-httpd.json
	dcos marathon app add examples/apps/bridge-nginx.json
	cat examples/config/pool-http.json |sed -e "s,count\": 1,count\": $(POOL_COUNT),g" \
		>examples/config/pool-http-${POOL_COUNT}.json
	dcos edgelb config examples/config/pool-http-${POOL_COUNT}.json
	# TODO: curl the l7 route?
	rm examples/config/pool-http-${POOL_COUNT}.json

# test-http and test-https are mutually exclusive since they share the same marathon app name
test-https: maybe-install-dcos-cli
	until dcos edgelb ping; do sleep 1; done
	dcos marathon app add examples/apps/host-httpd.json
	cat examples/config/pool-https.json |sed -e "s,count\": 1,count\": $(POOL_COUNT),g" \
		>examples/config/pool-https-${POOL_COUNT}.json
	dcos edgelb config examples/config/pool-https-${POOL_COUNT}.json
	# TODO: curl the l7 route?
	rm examples/config/pool-https-${POOL_COUNT}.json

reset-test-all: clean uninstall compile push install test-https

############## Includes
# Include all .mk files so you can have your own local configurations
include $(wildcard *.mk)
