package dependency

import (
	"log"

	mesos_v1 "github.com/mesosphere/dcos-edge-lb/go-mesos-operator/include/mesos/v1"
	"github.com/pkg/errors"
)

type DCOSService struct {
	Type string
	Host string
	Port string
}

type MesosTask struct {
	Task  *mesos_v1.Task
	Agent *mesos_v1.AgentInfo
}

type MesosQuery struct {
	stopCh chan struct{}
}

func NewMesosQuery() *MesosQuery {
	stop := make(chan struct{})

	mq := MesosQuery{
		stopCh: stop,
	}

	return &mq
}

func (d *MesosQuery) Fetch(clients *ClientSet, opts *QueryOptions) (interface{}, *ResponseMetadata, error) {
	select {
	case <-d.stopCh:
		clients.mesos.unsubscribe(d.String())
		return "", nil, ErrStopped
	case <-clients.mesos.subscribe(d.String()):
		payload := clients.mesos.read()

		if payload.Err != nil {
			log.Printf("[DEBUG] (mesos) mesosquery payload error: %s", payload.Err)
			return "", nil, errors.Wrap(payload.Err, d.String())
		}

		// Copied from respWithMetadata()
		//
		// The LastIndex is meant to be a counter that tells the difference
		// between versions of data, but we just stick in the payload id
		// which is a random uuid which required a little hacking in
		// watch/view.go
		return payload, &ResponseMetadata{
			LastContact: 0,
			LastIndex:   payload.id,
		}, nil
	}
}

// CanShare returns a boolean if this dependency is shareable.
func (d *MesosQuery) CanShare() bool {
	return false
}

// Stop halts the dependency's fetch function.
func (d *MesosQuery) Stop() {
	close(d.stopCh)
}

// String returns the human-friendly version of this dependency.
func (d *MesosQuery) String() string {
	// This function is the one that's used to track the task! if this changes
	// then this will be killed!
	return "mesosquery"

}
