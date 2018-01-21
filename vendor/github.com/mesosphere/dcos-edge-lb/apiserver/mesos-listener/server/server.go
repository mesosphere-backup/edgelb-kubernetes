package server

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/mesosphere/dcos-edge-lb/apiserver/mesos-listener/listener"
	"github.com/mesosphere/dcos-edge-lb/apiserver/mesos-listener/util"
	mesos_v1 "github.com/mesosphere/dcos-edge-lb/go-mesos-operator/include/mesos/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type subscriber chan struct{}

type snapshotServer struct {
	ml *listener.MesosListener

	subs   map[subscriber]struct{}
	subMut sync.RWMutex
}

var logger = util.Logger

// Serve starts the server.
func Serve(ctx context.Context, mesoslis *listener.MesosListener, netlis net.Listener) error {
	gs := grpc.NewServer()
	ss := &snapshotServer{
		ml:   mesoslis,
		subs: make(map[subscriber]struct{}),
	}
	mesos_v1.RegisterSnapshotSubscribeServer(gs, ss)
	reflection.Register(gs)
	go ss.relaySignal(ctx)
	return gs.Serve(netlis)
}

func (s *snapshotServer) relaySignal(ctx context.Context) {
	for {
		select {
		case <-s.ml.SignalC:
			s.subMut.RLock()
			relaySignalOnce(s.subs)
			s.subMut.RUnlock()
		case <-ctx.Done():
			logger.Debug("(server) signal relay terminated")
			return
		}
	}
}

func relaySignalOnce(subscribers map[subscriber]struct{}) {
	logger.WithField("numSubscribers", len(subscribers)).Debug("(server) relaying signal")
	for subr := range subscribers {
		select {
		case subr <- struct{}{}:
			// noop
		default:
			// noop for nonblocking
		}
	}
}

func (s *snapshotServer) StreamSnapshot(_ *mesos_v1.SnapshotRequest, stream mesos_v1.SnapshotSubscribe_StreamSnapshotServer) error {
	// I'm not sure where the error in this function go, I think they are
	// sent to the client?

	subr := mkSubscriber()
	s.subscribe(subr)
	defer s.unsubscribe(subr)

	for {
		if err := s.streamSnapshotOnce(stream, subr); err != nil {
			logger.WithField("subscriber", subr).Error("(server) " + err.Error())
			return err
		}
	}
}

func (s *snapshotServer) streamSnapshotOnce(stream mesos_v1.SnapshotSubscribe_StreamSnapshotServer, subr subscriber) error {
	snap, snapExists, err := s.ml.Get()
	if err != nil {
		return fmt.Errorf("failed to get snapshot: %s", err)
	}
	if err := s.maybeSendSnapshot(stream, snap, snapExists); err != nil {
		return fmt.Errorf("failed to send snapshot: %s", err)
	}
	select {
	case <-subr:
		return nil
	case <-stream.Context().Done():
		return fmt.Errorf("context closed")
	}
}

func mkSubscriber() subscriber {
	// Have buffer size of 1 so there can always be an extra signal queued up
	// to indicate something has changed since last checked.
	return make(chan struct{}, 1)
}

func (s *snapshotServer) subscribe(subr subscriber) {
	s.subMut.Lock()
	defer s.subMut.Unlock()

	s.subs[subr] = struct{}{}
	logger.WithFields(logrus.Fields{
		"subscriber":     subr,
		"numSubscribers": len(s.subs),
	}).Debug("(server) subscribe")
}

func (s *snapshotServer) unsubscribe(subr subscriber) {
	s.subMut.Lock()
	defer s.subMut.Unlock()

	close(subr)
	delete(s.subs, subr)
	logger.WithFields(logrus.Fields{
		"subscriber":     subr,
		"numSubscribers": len(s.subs),
	}).Debug("(server) unsubscribe")
}

func (s *snapshotServer) maybeSendSnapshot(stream mesos_v1.SnapshotSubscribe_StreamSnapshotServer, snap mesos_v1.FrameworkSnapshot, snapExists bool) error {
	if !snapExists {
		logger.Info("(server) snapshot has not been initialized, not sending")
		return nil
	}
	logger.Debug("(server) sending snapshot")
	return stream.Send(&snap)
}
