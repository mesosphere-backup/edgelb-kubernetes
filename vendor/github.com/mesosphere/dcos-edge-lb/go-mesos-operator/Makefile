GOREPO = github.com/mesosphere/dcos-edge-lb/go-mesos-operator

all: compile

compile: proto

clean: clean-proto

clean-proto:
	rm -rf include/mesos

# Uses Mesos 1.1.0 proto
# a44b077ea0df54b77f05550979e1e97f39b15873
#
# Must be built on linux as protobuf generates different files there.
# The output also may change depending on what version of golang is being used.
proto: clean-proto
	cp -r include/mesos_original include/mesos
	cp -rn include/addons/* include/mesos
	sed -i'.sedbak' -e 's|^import "mesos|import "${GOREPO}/include/mesos|' \
		`find include/mesos -name '*.proto'`
	find include/mesos -name '*.sedbak' | xargs rm -f
	cd ${GOPATH}/src && \
		protoc --go_out=plugins=grpc:. ${GOREPO}/include/mesos/v1/*.proto && \
		protoc --go_out=. ${GOREPO}/include/mesos/v1/master/*.proto && \
		protoc --go_out=. ${GOREPO}/include/mesos/v1/maintenance/*.proto && \
		protoc --go_out=. ${GOREPO}/include/mesos/v1/allocator/*.proto && \
		protoc --go_out=. ${GOREPO}/include/mesos/v1/quota/*.proto

debugrun:
	go run go-mesos-operator.go -debug ${EXTRA} -addr `dig @198.51.100.1 +short leader.mesos`:5050
