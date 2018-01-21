package dependency

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	mesos_v1 "github.com/mesosphere/dcos-edge-lb/go-mesos-operator/include/mesos/v1"
	mesos "github.com/mesosphere/dcos-edge-lb/go-mesos-operator/mesos"
	"google.golang.org/grpc"
)

// ClientSet is a collection of clients that dependencies use to communicate
// with remote services like Consul or Vault.
type ClientSet struct {
	sync.RWMutex

	mesos *mesosClient
}

type mesosClient struct {
	// The id changes when something about the snapshot has changed
	id uint64

	snap    mesos_v1.FrameworkSnapshot
	snapMut sync.RWMutex

	subscribeMut sync.RWMutex

	// Subscribers to receive updates on Mesos state
	//
	// Depends on subscribeMut
	subscribers map[string]chan struct{}
}

type MesosPayload struct {
	Snap mesos_v1.FrameworkSnapshot
	Err  error

	// The id from the mesosClient is copied when this is created.
	id uint64
}

// A bad function that keeps looping until it successfully returns.
func mustRandUint64() uint64 {
	var buf [16]byte
	var err error

	for {
		_, err = crand.Read(buf[:])
		if err == nil {
			break
		}
		log.Printf("[ERROR] (clients) mustRandUint64: %s", err)
		time.Sleep(time.Second)
	}

	return binary.BigEndian.Uint64(buf[:])
}

// Returns a channel that carries signals. A subscribe with the same id
// will result in the same channel being returned.
func (c *mesosClient) subscribe(id string) <-chan struct{} {
	// There actually should only be 1 listener with how consul-template works.
	// My impression of how this works is that consul-template will call a
	// function from template/func and all of these will call the same
	// mesosquery from dependency/mesos. My impression of how the "brain" works
	// is that since these all use the same thing, it'll actually only
	// have 1 listener, but we should check here. Maybe run a couple manual
	// tests that log/alert/panic when more than 1 listener exists. Also test
	// if there are multiple calls to the functions within the template.
	//
	// There are multiple calls to the Func in template/func. However, if they
	// have the same dependency (dependency/mesos in our case) then only 1 will
	// be run, which is what we want.

	var subC chan struct{}

	c.subscribeMut.Lock()

	if s, ok := c.subscribers[id]; ok {
		subC = s
	} else {
		log.Printf(fmt.Sprintf("[DEBUG] (clients) mesos new subscribe: %s", id))
		// Channel must have buffer size 1, that way we can have non-blocking
		// writes while still having having signals that alert whenever a change
		// has occurred since the subscriber has last checked.
		subC = make(chan struct{}, 1)

		// Seed it with an update so subscriber grabs initial state
		subC <- struct{}{}

		c.subscribers[id] = subC
	}
	c.subscribeMut.Unlock()
	return subC
}

func (c *mesosClient) unsubscribe(id string) {
	c.subscribeMut.Lock()
	delete(c.subscribers, id)
	c.subscribeMut.Unlock()
}

func (c *mesosClient) notify() {
	c.subscribeMut.RLock()
	for _, sub := range c.subscribers {
		select {
		case sub <- struct{}{}:
			// noop
		default:
			// noop for nonblocking
		}

	}
	c.subscribeMut.RUnlock()
}

func (c *mesosClient) read() MesosPayload {
	c.snapMut.RLock()
	sclone, err := mesos.CloneSnapshot(c.snap)
	mp := MesosPayload{
		Snap: sclone,
		Err:  err,
		id:   c.id,
	}
	c.snapMut.RUnlock()
	return mp
}

// NewClientSet creates a new client set that is ready to accept clients.
func NewClientSet() *ClientSet {
	return &ClientSet{}
}

func (c *ClientSet) CreateMesosClient(mesosInput string) {
	if mesosInput == "" {
		return
	}

	c.mesos = &mesosClient{
		// The id here must be initialized so that the listener gets something
		// that is guaranteed to be different from anything it has.
		id: mustRandUint64(),

		subscribers: make(map[string]chan struct{}),
	}

	go func() {
		if err := runMesosClient(c.mesos, mesosInput); err != nil {
			log.Fatalf("[FATAL] (clients) mesos client failed: %s", err)
		}
		log.Fatal("[FATAL] (clients) mesos client terminated")
	}()
}

func runMesosClient(mc *mesosClient, serverAddr string) error {
	conn, err := grpc.Dial(serverAddr, grpc.WithInsecure())
	if err != nil {
		return fmt.Errorf("did not connect: %s", err)
	}
	defer conn.Close()
	gc := mesos_v1.NewSnapshotSubscribeClient(conn)

	stream, err := gc.StreamSnapshot(context.Background(), &mesos_v1.SnapshotRequest{})
	if err != nil {
		return fmt.Errorf("could not stream: %s", err)
	}
	for {
		snapRef, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("bad stream: %s", err)
		}
		if snapRef == nil {
			return fmt.Errorf("nil snap")
		}

		snap := *snapRef

		mc.snapMut.Lock()
		mc.snap = snap
		mc.id = mustRandUint64()
		mc.notify()
		mc.snapMut.Unlock()
	}
	return nil
}

// Stop closes all idle connections for any attached clients.
func (c *ClientSet) Stop() {
	c.Lock()
	defer c.Unlock()

	// XXX: Close mesos connection
}
