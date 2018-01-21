package models

import (
	"strconv"
	"testing"
)

func TestMesosTaskID(t *testing.T) {
	podTaskID := "group_pod-overlay-mesos-id-unused-podname.instance-b5407709-df83-11e7-b5eb-4e2c5443c397.pod-overlay-mesos-id"
	svcID, cName := MesosTaskIDToMarathonServiceIDContainerName(podTaskID)
	assertEqual(t, "/group/pod-overlay-mesos-id-unused-podname", svcID)
	assertEqual(t, "pod-overlay-mesos-id", cName)

	appTaskID := "default-svc.902a55cd-df84-11e7-b5eb-4e2c5443c397"
	svcID, cName = MesosTaskIDToMarathonServiceIDContainerName(appTaskID)
	assertEqual(t, "/default-svc", svcID)
	assertEqual(t, "", cName)

	frameworkTaskID := "902a55cd-df84-11e7-b5eb-4e2c5443c397"
	svcID, cName = MesosTaskIDToMarathonServiceIDContainerName(frameworkTaskID)
	assertEqual(t, "", svcID)
	assertEqual(t, "", cName)
}

func TestPoolFromV1(t *testing.T) {
	v1server := makeV1ServerExact("marathon", "myapp", "web")
	v1pool := makeV1Pool(&v1server)

	if pool, err := V2PoolFromV1(&v1pool); err != nil {
		t.Fatal(err)
	} else {
		assertEqual(t, "marathon", pool.Haproxy.Backends[0].Services[0].Mesos.FrameworkName)
		assertEqual(t, "myapp", pool.Haproxy.Backends[0].Services[0].Mesos.TaskName)
		assertEqual(t, "web", pool.Haproxy.Backends[0].Services[0].Endpoint.PortName)
	}
}

func TestV2ServiceFromV1Server(t *testing.T) {
	v1server := makeV1ServerExact("marathon", "myapp", "web")
	if svc, err := V2ServiceFromV1Server(&v1server); err != nil {
		t.Fatal(err)
	} else {
		assertEqual(t, "marathon", svc.Mesos.FrameworkName)
		assertEqual(t, "myapp", svc.Mesos.TaskName)
		assertEqual(t, "web", svc.Endpoint.PortName)
	}

	v1server = makeV1ServerExact("cassandra", "myapp", "web")
	if svc, err := V2ServiceFromV1Server(&v1server); err != nil {
		t.Fatal(err)
	} else {
		assertEqual(t, "cassandra", svc.Mesos.FrameworkName)
		assertEqual(t, "myapp", svc.Mesos.TaskName)
		assertEqual(t, "web", svc.Endpoint.PortName)
	}

	v1server = makeV1ServerExact("cassandra", "myapp", "")
	v1server.Framework.Match = V1MatchREGEX
	v1server.Task.Match = V1MatchREGEX
	v1server.Port.All = true
	if svc, err := V2ServiceFromV1Server(&v1server); err != nil {
		t.Fatal(err)
	} else {
		assertEqual(t, "cassandra", svc.Mesos.FrameworkNamePattern)
		assertEqual(t, "myapp", svc.Mesos.TaskNamePattern)
		assertTrue(t, "allPorts", svc.Endpoint.AllPorts)
	}

	v1server = makeV1ServerExact("cassandra", "myapp", "")
	v1server.Type = V1ServerTypeVIP
	v1server.Port.Vip = "/myvip:9999"
	if svc, err := V2ServiceFromV1Server(&v1server); err != nil {
		t.Fatal(err)
	} else {
		assertEqual(t, "cassandra", svc.Mesos.FrameworkName)
		assertEqual(t, "myapp", svc.Mesos.TaskName)
		assertTrue(t, "endpointTypeAddress", svc.Endpoint.Type == V2EndpointTypeADDRESS)
		assertEqual(t, "myvip.cassandra.l4lb.thisdcos.directory", svc.Endpoint.Address)
		assertEqual(t, "9999", strconv.Itoa(int(svc.Endpoint.Port)))
	}

	v1server = makeV1ServerExact("cassandra", "myapp", "")
	v1server.Type = V1ServerTypeVIP
	v1server.Port.Vip = "master.mesos:5050"
	if svc, err := V2ServiceFromV1Server(&v1server); err != nil {
		t.Fatal(err)
	} else {
		assertEqual(t, "cassandra", svc.Mesos.FrameworkName)
		assertEqual(t, "myapp", svc.Mesos.TaskName)
		assertTrue(t, "endpointTypeAddress", svc.Endpoint.Type == V2EndpointTypeADDRESS)
		assertEqual(t, "master.mesos", svc.Endpoint.Address)
		assertEqual(t, "5050", strconv.Itoa(int(svc.Endpoint.Port)))
	}

	v1server = makeV1ServerExact("cassandra", "myapp", "")
	v1server.Type = V1ServerTypeVIP
	v1server.Port.Vip = "master.mesos"
	if svc, err := V2ServiceFromV1Server(&v1server); err != nil {
		t.Fatal(err)
	} else {
		assertEqual(t, "cassandra", svc.Mesos.FrameworkName)
		assertEqual(t, "myapp", svc.Mesos.TaskName)
		assertTrue(t, "endpointTypeAddress", svc.Endpoint.Type == V2EndpointTypeADDRESS)
		assertEqual(t, "master.mesos", svc.Endpoint.Address)
		assertEqual(t, "-1", strconv.Itoa(int(svc.Endpoint.Port)))
	}
}

func assertTrue(t *testing.T, name string, test bool) {
	if !test {
		t.Errorf("\nexpected %s to be true, but got false", name)
	}
}

func assertEqual(t *testing.T, expected, actual string) {
	if expected != actual {
		t.Errorf("\nexp: %t\nact: %t", expected, actual)
	}
}

func makeV1ServerExact(framework, task, port string) V1Server {
	var v1server V1Server
	v1server.UnmarshalBinary([]byte(`{}`))
	v1server.Framework.Value = framework
	v1server.Task.Value = task
	v1server.Port.Name = port
	return v1server
}

func makeV1Pool(server *V1Server) V1Pool {
	var v1pool V1Pool
	v1pool.UnmarshalBinary([]byte(`{}`))
	var v1backend V1Backend
	v1backend.UnmarshalBinary([]byte(`{}`))
	v1backend.Servers = append(v1backend.Servers, server)
	v1pool.Haproxy.Backends = append(v1pool.Haproxy.Backends, &v1backend)
	return v1pool
}
