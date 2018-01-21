package listener

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	mrand "math/rand"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	dcos "github.com/mesosphere/dcos-edge-lb/apiserver/dcos"
	"github.com/mesosphere/dcos-edge-lb/apiserver/mesos-listener/util"
	mesos_v1 "github.com/mesosphere/dcos-edge-lb/go-mesos-operator/include/mesos/v1"
	mesos "github.com/mesosphere/dcos-edge-lb/go-mesos-operator/mesos"
)

// XXX Currently we have a set of timers associated with tasks in order
//   to keep track of which tasks to keep alive. In addition, we keep
//   around any frameworks/agents that are associated with these tasks.
//   This approach basically keeps around a fake view of cluster state.
//   An alternative approach is to maintain 2 completely separate state,
//   the current one, and one that is completely made up of timers. This
//   approach has less logic when it comes to maintaining state, and instead
//   pushes the complexity to template render time, which is when we
//   reconcile the 2 states.

// XXX Another thing to consider is that we might be able to wait for
//   an AGENT_REMOVED message instead of using a timeout. The issue with
//   that is that we'd need some signal that we receive that message
//   from go-mesos-operater, which currently abstracts away the individual
//   messages that are sent. Possibly hook up a channel that alerts on
//   agent removal events? Or even simpler, just have a field in the
//   state that is passed from go-mesos-operator include a special
//   slice that lists agents that have been removed in that state change.
//
// Mesos is configured to wait for 10 minutes for an agent to reconnect. Just
// to be safe, we will wait longer than that.
//
// Time is in seconds
//// XXX FOR DEBUG PURPOSES, maybe make all of these configurable?
//const failoverTimeout int64 = 30
const failoverTimeout int64 = 60 * 11

const timerCheckInterval time.Duration = time.Second * 2
const listenerCrashWait time.Duration = time.Millisecond * 100
const maxCrashWait time.Duration = time.Second * 10

// MesosListener is the listener state
type MesosListener struct {
	// The state of mesos
	snap mesos_v1.FrameworkSnapshot
	// A guard for mutating snap
	snapMut sync.RWMutex
	// Whether or not the first snap has been retrieved
	snapExists bool

	// This is set when a failover is detected. It is cleared on the
	// next update.
	//
	// Depends on snapMut
	leaderFailover bool

	// A non-blocking message is sent when snap is updated. There may also
	// be messages even when no update to the snapshot has been made.
	SignalC chan struct{}

	// Map from mesos ID to Unix time
	//
	// This map also depends on snapMut
	taskTimers map[string]int64

	// These maps track the references to frameworks/agents that are
	// carried over as dependencies of tasks. These should never
	// point to "real" values, only "carried" ones.
	//
	// These are closely associated with taskTimers, so they should roughly
	// appear wherever those do
	//
	// These maps also depend on snapMut
	carriedFwRef map[string]int
	carriedAgRef map[string]int
}

var logger = util.Logger

// NewMesosListener creates a new mesos listener
func NewMesosListener() *MesosListener {
	return &MesosListener{
		SignalC:    make(chan struct{}, 1),
		taskTimers: make(map[string]int64),
	}
}

// Listen to mesos
func (ml *MesosListener) Listen(ctx context.Context, addr string, prot mesos.Protocol, mkClient func() dcos.Client) {
	// XXX Context is currently unused. It may be fairly important to implement it here.

	go ml.runListener(addr, prot, mkClient)

	go ml.runTimer()
}

func (ml *MesosListener) runTimer() {
	for {
		ml.snapMut.Lock()
		ml.checkTimers()
		ml.snapMut.Unlock()
		time.Sleep(timerCheckInterval)
	}
}

func (ml *MesosListener) runListener(addr string, prot mesos.Protocol, mkClient func() dcos.Client) {
	// XXX As future work, instead of assuming every crash here is leader
	//   failure, have a more fine grained error scheme so we can decide
	//   when to crash and when to just carry on.

	seed := int64(mustRandUint64())
	mrand.Seed(seed)
	wait := listenerCrashWait
	logger.WithFields(logrus.Fields{
		"failoverTimeout": failoverTimeout,
		"seed":            seed,
		"wait":            wait,
	}).Info("(listener) mesos listen init")

	for {
		backoffExpired := time.Now().Add(maxCrashWait)
		ctx := context.Background()
		if err := mesos.NewFrameworkListener(ctx, addr, prot, mkClient, ml.mkUpdateFn()); err != nil {
			logger.WithError(err).Info("(listener) mesos listener crashed, assuming mesos leader failover")
		}

		if time.Now().After(backoffExpired) {
			logger.Info("(listener) mesos listener backoff expired")
			wait = listenerCrashWait
		}

		ml.snapMut.Lock()
		ml.leaderFailover = true
		ml.snapMut.Unlock()

		logger.WithField("wait", wait).Info("(listener) mesos listener sleeping")
		time.Sleep(wait)
		wait *= 2
		if wait > maxCrashWait {
			wait = maxCrashWait
		}
	}
}

func (ml *MesosListener) mkUpdateFn() func(ctx context.Context, snapshot mesos_v1.FrameworkSnapshot, err error) {
	return func(ctx context.Context, snapshot mesos_v1.FrameworkSnapshot, err error) {
		// Context is currently unused.

		if err != nil {
			logger.WithError(err).Error("(listener) mesos operator callback failed")
			return
		}
		ml.update(snapshot)
	}
}

// Get snapshot
func (ml *MesosListener) Get() (mesos_v1.FrameworkSnapshot, bool, error) {
	var snapExists bool

	ml.snapMut.RLock()
	snapExists = ml.snapExists
	snapCopy, err := mesos.CloneSnapshot(ml.snap)
	ml.snapMut.RUnlock()

	if err != nil {
		return mesos_v1.FrameworkSnapshot{}, false, err
	}
	return snapCopy, snapExists, nil
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
		logger.WithError(err).Error("(listener) mustRandUint64")
		time.Sleep(time.Second)
	}

	return binary.BigEndian.Uint64(buf[:])
}

func (ml *MesosListener) notify() {
	logger.Debug("(listener) mesos notify")
	select {
	case ml.SignalC <- struct{}{}:
		// noop
	default:
		// noop for non-blocking
	}
}

func (ml *MesosListener) update(snap mesos_v1.FrameworkSnapshot) {
	// The passed in snap is modified as it is assumed that it is given
	// a personal copy.

	ml.snapMut.Lock()
	defer ml.snapMut.Unlock()
	logger.Debug("(listener) mesos update: running update")
	ml.snapExists = true
	ml.notify()

	// We check timers before picking out tasks to carry over. Otherwise,
	// expired tasks will get carried over before they can be deleted.
	ml.checkTimers()

	var carriedTasks []string

	// Pick out the tasks to carry over
	for tid, oldTask := range ml.snap.Tasks {
		fid := oldTask.GetFrameworkId().GetValue()
		agid := oldTask.GetAgentId().GetValue()
		if _, ok := snap.Tasks[tid]; ok {
			// The task already exists in the new snapshot
			logger.WithField("tid", tid).Debug("(listener) mesos update task exists in new snapshot")
			ml.maybeClearTimer(tid, fid, agid)
			continue
		}
		if _, ok := snap.Agents[agid]; ok {
			// The task does not exist in the new snapshot yet the
			// old agent has reconnected, so we delete the old task.
			logger.WithField("tid", tid).Debug("(listener) mesos update task deleted or agent reconnected without task")
			ml.maybeClearTimer(tid, fid, agid)
			continue
		}
		if !ml.hasTimer(tid) && !ml.leaderFailover {
			continue
		}
		carriedTasks = append(carriedTasks, tid)
	}

	// Reset carry maps, so we don't have to keep track of which ones have
	// been replaced by real values.
	ml.fwRefsReset()
	ml.agRefsReset()

	// Update carry maps
	//
	// The reason this is a separate step is because we want to count the
	// number of references to each carried value, which we have to do
	// before modifying the snapshot.
	for _, tid := range carriedTasks {
		oldTask := ml.snap.Tasks[tid]

		fid := oldTask.GetFrameworkId().GetValue()
		ml.fwRefUp(fid)
		agid := oldTask.GetAgentId().GetValue()
		ml.agRefUp(agid)
	}

	// Modify the new snapshot with carried over values
	//
	// The reason this is a separate step is because we don't want to modify
	// the new snapshot while the new snapshot is used to compute which
	// values are to be carried over.
	for _, tid := range carriedTasks {
		logger.WithField("tid", tid).Debug("(listener) mesos update carrying task")
		snap.Tasks[tid] = ml.snap.Tasks[tid]
	}
	for fid := range ml.carriedFwRef {
		logger.WithField("fid", fid).Debug("(listener) mesos update carrying framework")
		snap.Frameworks[fid] = ml.snap.Frameworks[fid]
	}
	for agid := range ml.carriedAgRef {
		logger.WithField("agid", agid).Debug("(listener) mesos update carrying agent")
		snap.Agents[agid] = ml.snap.Agents[agid]
	}

	// Replace the old snapshot with new one
	ml.snap.Frameworks = snap.Frameworks
	ml.snap.Agents = snap.Agents
	ml.snap.Tasks = snap.Tasks

	if ml.leaderFailover {
		now := time.Now().Unix()
		newTimeout := now + failoverTimeout
		for _, tid := range carriedTasks {
			ml.taskTimers[tid] = newTimeout
		}
	}

	// Leader failover is always consumed at the end of its update cycle
	ml.leaderFailover = false
}

func (ml *MesosListener) fwRefsReset() {
	// Assumes that it's in a locked context.
	// Depends on snapMut.

	logger.Debug("(listener) mesos fw refs reset")
	ml.carriedFwRef = make(map[string]int)
}

func (ml *MesosListener) agRefsReset() {
	// Assumes that it's in a locked context.
	// Depends on snapMut.

	logger.Debug("(listener) mesos ag refs reset")
	ml.carriedAgRef = make(map[string]int)
}

func (ml *MesosListener) fwRefUp(fid string) {
	// Assumes that it's in a locked context.
	// Depends on snapMut.

	logger.WithField("fid", fid).Debug("(listener) mesos fw ref up")
	refUpHelper(ml.carriedFwRef, fid)
}

func (ml *MesosListener) agRefUp(agid string) {
	// Assumes that it's in a locked context.
	// Depends on snapMut.

	logger.WithField("agid", agid).Debug("(listener) mesos ag ref up")
	refUpHelper(ml.carriedAgRef, agid)
}

func refUpHelper(refMap map[string]int, id string) {
	if count, ok := refMap[id]; ok && count <= 0 {
		logger.WithFields(logrus.Fields{
			"id":    id,
			"count": count,
		}).Error("(listener) mesos ref up TOO LOW")
	}

	if count, ok := refMap[id]; !ok {
		logger.WithField("id", id).Debug("(listener) mesos ref up: creating")
		refMap[id] = 1
	} else {
		refMap[id] = count + 1
	}
}

func (ml *MesosListener) fwRefDown(fid string) {
	// Assumes that it's in a locked context.
	// Depends on snapMut.

	logger.WithField("fid", fid).Debug("(listener) mesos fw ref down")
	refDownHelper(ml.carriedFwRef, fid)
}

func (ml *MesosListener) agRefDown(agid string) {
	// Assumes that it's in a locked context.
	// Depends on snapMut.

	logger.WithField("agid", agid).Debug("(listener) mesos ag ref down")
	refDownHelper(ml.carriedAgRef, agid)
}

func refDownHelper(refMap map[string]int, id string) {
	// XXX Should write a test that ensures that this never drops below 1

	count := refMap[id]
	if count == 1 {
		logger.WithField("id", id).Debug("(listener) mesos ref down: deleting")
		delete(refMap, id)
		return
	}
	refMap[id] = count - 1

	if count, ok := refMap[id]; ok && count <= 0 {
		logger.WithFields(logrus.Fields{
			"id":    id,
			"count": count,
		}).Error("(listener) mesos ref down TOO LOW")
	}
}

func (ml *MesosListener) hasTimer(taskID string) bool {
	_, ok := ml.taskTimers[taskID]
	return ok
}

// Only clears timer if timer exists
func (ml *MesosListener) maybeClearTimer(taskID, frameworkID, agentID string) {
	if _, ok := ml.taskTimers[taskID]; ok {
		ml.clearTimer(taskID, frameworkID, agentID)
	}
}

func (ml *MesosListener) clearTimer(taskID, frameworkID, agentID string) {
	// Assumes that it's in a locked context.
	// Depends on snapMut.

	logger.WithField("tid", taskID).Debug("(listener) mesos clearing task timer")
	ml.fwRefDown(frameworkID)
	ml.agRefDown(agentID)
	delete(ml.taskTimers, taskID)
}

// Scan through the active timers and clear the timer and delete associated
// task/framework/agent if it timed out. Triggers an update if
// something changes.
func (ml *MesosListener) checkTimers() {
	// Assumes that it's in a locked context.
	// Depends on snapMut.

	now := time.Now().Unix()
	changed := false
	for tid, endtime := range ml.taskTimers {
		if endtime > now {
			continue
		}
		fid := ml.snap.Tasks[tid].GetFrameworkId().GetValue()
		agid := ml.snap.Tasks[tid].GetAgentId().GetValue()
		ml.clearTimer(tid, fid, agid)

		logger.WithField("tid", tid).Debug("(listener) mesos timer expired deleting task")
		delete(ml.snap.Tasks, tid)
		if _, ok := ml.carriedFwRef[fid]; !ok {
			logger.WithField("fid", fid).Debug("(listener) mesos timer deleting unreferenced framework")
			delete(ml.snap.Frameworks, fid)
		}
		if _, ok := ml.carriedAgRef[agid]; !ok {
			logger.WithField("agid", agid).Debug("(listener) mesos timer deleting unreferenced agent")
			delete(ml.snap.Agents, agid)
		}
		changed = true
	}

	if changed {
		logger.Debug("(listener) mesos check timer triggered change")
		ml.notify()
	}
}
