package listener

import (
	"fmt"
	"runtime/debug"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/davecgh/go-spew/spew"
	"github.com/golang/protobuf/proto"
	mesos_v1 "github.com/mesosphere/dcos-edge-lb/go-mesos-operator/include/mesos/v1"
	mesos "github.com/mesosphere/dcos-edge-lb/go-mesos-operator/mesos"
)

type snapSpec struct {
	tasks      []taskSpec
	frameworks []frameworkSpec
	agents     []agentSpec
}

type taskSpec struct {
	id   string
	fid  string
	agid string
}

type frameworkSpec struct {
	id string
}

type agentSpec struct {
	id string
}

func init() {
	logger.Level = logrus.DebugLevel
}

func TestCheckTimers(t *testing.T) {
	var signals int
	ml := NewMesosListener()

	tid1 := "tid1"
	fid1 := "fid1"
	agid1 := "agid1"

	tid2 := "tid2"
	fid2 := "fid2"
	agid2 := "agid2"

	tid3 := "tid3"

	spec := snapSpec{
		tasks: []taskSpec{
			{id: tid1, fid: fid1, agid: agid1},
			{id: tid2, fid: fid2, agid: agid2},
			{id: tid3, fid: fid1, agid: agid1},
		},
		frameworks: []frameworkSpec{
			{id: fid1},
			{id: fid2},
		},
		agents: []agentSpec{
			{id: agid1},
			{id: agid2},
		},
	}

	var snapErr error
	ml.snap, snapErr = mkSnap(spec)
	if snapErr != nil {
		t.Fatal(snapErr)
	}
	spew.Dump(ml.snap)

	ml.taskTimers[tid1] = time.Now().Unix()
	ml.taskTimers[tid2] = time.Now().Unix() + 999
	ml.taskTimers[tid3] = time.Now().Unix() + 999
	ml.carriedFwRef = map[string]int{fid1: 2, fid2: 1}
	ml.carriedAgRef = map[string]int{agid1: 2, agid2: 1}

	// Clear 1
	ml.checkTimers()
	signals = countAndConsumeSignals(ml)
	assert(t, len(ml.taskTimers) == 2, spew.Sdump(ml.taskTimers))
	assert(t, len(ml.carriedFwRef) == 2, spew.Sdump(ml.carriedFwRef))
	assert(t, len(ml.carriedAgRef) == 2, spew.Sdump(ml.carriedAgRef))
	assert(t, signals > 0, spew.Sdump(signals))
	assert(t, len(ml.snap.Tasks) == 2, spew.Sdump(ml.snap))
	assert(t, len(ml.snap.Agents) == 2, spew.Sdump(ml.snap))
	assert(t, len(ml.snap.Frameworks) == 2, spew.Sdump(ml.snap))

	// Check that nothing changes
	ml.checkTimers()
	signals = countAndConsumeSignals(ml)
	assert(t, len(ml.taskTimers) == 2, spew.Sdump(ml.taskTimers))
	assert(t, len(ml.carriedFwRef) == 2, spew.Sdump(ml.carriedFwRef))
	assert(t, len(ml.carriedAgRef) == 2, spew.Sdump(ml.carriedAgRef))
	assert(t, signals == 0, spew.Sdump(signals))
	assert(t, len(ml.snap.Tasks) == 2, spew.Sdump(ml.snap))
	assert(t, len(ml.snap.Agents) == 2, spew.Sdump(ml.snap))
	assert(t, len(ml.snap.Frameworks) == 2, spew.Sdump(ml.snap))

	// Clear all
	ml.taskTimers[tid2] = time.Now().Unix()
	ml.taskTimers[tid3] = time.Now().Unix()
	ml.checkTimers()
	signals = countAndConsumeSignals(ml)
	assert(t, len(ml.taskTimers) == 0, spew.Sdump(ml.taskTimers))
	assert(t, len(ml.carriedFwRef) == 0, spew.Sdump(ml.carriedFwRef))
	assert(t, len(ml.carriedAgRef) == 0, spew.Sdump(ml.carriedAgRef))
	assert(t, signals > 0, spew.Sdump(signals))
	assert(t, len(ml.snap.Tasks) == 0, spew.Sdump(ml.snap))
	assert(t, len(ml.snap.Agents) == 0, spew.Sdump(ml.snap))
	assert(t, len(ml.snap.Frameworks) == 0, spew.Sdump(ml.snap))

	// Check that nothing changes
	ml.checkTimers()
	signals = countAndConsumeSignals(ml)
	assert(t, len(ml.taskTimers) == 0, spew.Sdump(ml.taskTimers))
	assert(t, len(ml.carriedFwRef) == 0, spew.Sdump(ml.carriedFwRef))
	assert(t, len(ml.carriedAgRef) == 0, spew.Sdump(ml.carriedAgRef))
	assert(t, signals == 0, spew.Sdump(signals))
	assert(t, len(ml.snap.Tasks) == 0, spew.Sdump(ml.snap))
	assert(t, len(ml.snap.Agents) == 0, spew.Sdump(ml.snap))
	assert(t, len(ml.snap.Frameworks) == 0, spew.Sdump(ml.snap))
}

func countAndConsumeSignals(ml *MesosListener) int {
	signals := 0
	for {
		select {
		case <-ml.SignalC:
			signals++
		default:
			return signals
		}
	}
}

func TestNoUpdate(t *testing.T) {
	ml := NewMesosListener()

	tid1 := "tid1"
	fid1 := "fid1"
	agid1 := "agid1"

	tid2 := "tid2"
	fid2 := "fid2"
	agid2 := "agid2"

	tid3 := "tid3"

	spec := snapSpec{
		tasks: []taskSpec{
			{id: tid1, fid: fid1, agid: agid1},
			{id: tid2, fid: fid2, agid: agid2},
			{id: tid3, fid: fid1, agid: agid1},
		},
		frameworks: []frameworkSpec{
			{id: fid1},
			{id: fid2},
		},
		agents: []agentSpec{
			{id: agid1},
			{id: agid2},
		},
	}

	snap, snapErr := mkSnap(spec)
	if snapErr != nil {
		t.Fatal(snapErr)
	}
	spew.Dump(snap)

	emptySnap, emptySnapErr := mkSnap(snapSpec{})
	if emptySnapErr != nil {
		t.Fatal(emptySnapErr)
	}

	// Setup
	if s, err := mesos.CloneSnapshot(snap); err != nil {
		t.Fatal(err)
	} else {
		ml.update(s)
	}
	assert(t, proto.Equal(&snap, &ml.snap), fmt.Sprintf("\nexpected: %s\ngot:      %s", spew.Sdump(snap), spew.Sdump(ml.snap)))
	assert(t, len(ml.taskTimers) == 0, spew.Sdump(ml.taskTimers))
	assert(t, len(ml.carriedFwRef) == 0, spew.Sdump(ml.carriedFwRef))
	assert(t, len(ml.carriedAgRef) == 0, spew.Sdump(ml.carriedAgRef))
	assert(t, ml.carriedFwRef[fid1] == 0, spew.Sdump(ml.carriedFwRef))
	assert(t, ml.carriedFwRef[fid2] == 0, spew.Sdump(ml.carriedFwRef))
	assert(t, ml.carriedAgRef[agid1] == 0, spew.Sdump(ml.carriedAgRef))
	assert(t, ml.carriedAgRef[agid2] == 0, spew.Sdump(ml.carriedAgRef))

	// No change
	if s, err := mesos.CloneSnapshot(snap); err != nil {
		t.Fatal(err)
	} else {
		ml.update(s)
	}
	assert(t, proto.Equal(&snap, &ml.snap), fmt.Sprintf("\nexpected: %s\ngot:      %s", spew.Sdump(snap), spew.Sdump(ml.snap)))
	assert(t, len(ml.taskTimers) == 0, spew.Sdump(ml.taskTimers))
	assert(t, len(ml.carriedFwRef) == 0, spew.Sdump(ml.carriedFwRef))
	assert(t, len(ml.carriedAgRef) == 0, spew.Sdump(ml.carriedAgRef))
	assert(t, ml.carriedFwRef[fid1] == 0, spew.Sdump(ml.carriedFwRef))
	assert(t, ml.carriedFwRef[fid2] == 0, spew.Sdump(ml.carriedFwRef))
	assert(t, ml.carriedAgRef[agid1] == 0, spew.Sdump(ml.carriedAgRef))
	assert(t, ml.carriedAgRef[agid2] == 0, spew.Sdump(ml.carriedAgRef))

	// Failover, all gone, no change
	ml.leaderFailover = true
	if s, err := mesos.CloneSnapshot(emptySnap); err != nil {
		t.Fatal(err)
	} else {
		ml.update(s)
	}
	assert(t, !ml.leaderFailover, spew.Sdump(ml.leaderFailover))

	assert(t, proto.Equal(&snap, &ml.snap), fmt.Sprintf("\nexpected: %s\ngot:      %s", spew.Sdump(snap), spew.Sdump(ml.snap)))
	assert(t, len(ml.taskTimers) == 3, spew.Sdump(ml.taskTimers))
	assert(t, len(ml.carriedFwRef) == 2, spew.Sdump(ml.carriedFwRef))
	assert(t, len(ml.carriedAgRef) == 2, spew.Sdump(ml.carriedAgRef))
	assert(t, ml.carriedFwRef[fid1] == 2, spew.Sdump(ml.carriedFwRef))
	assert(t, ml.carriedFwRef[fid2] == 1, spew.Sdump(ml.carriedFwRef))
	assert(t, ml.carriedAgRef[agid1] == 2, spew.Sdump(ml.carriedAgRef))
	assert(t, ml.carriedAgRef[agid2] == 1, spew.Sdump(ml.carriedAgRef))

	// No change
	if s, err := mesos.CloneSnapshot(emptySnap); err != nil {
		t.Fatal(err)
	} else {
		ml.update(s)
	}
	assert(t, proto.Equal(&snap, &ml.snap), fmt.Sprintf("\nexpected: %s\ngot:      %s", spew.Sdump(snap), spew.Sdump(ml.snap)))

	assert(t, len(ml.taskTimers) == 3, spew.Sdump(ml.taskTimers))
	assert(t, len(ml.carriedFwRef) == 2, spew.Sdump(ml.carriedFwRef))
	assert(t, len(ml.carriedAgRef) == 2, spew.Sdump(ml.carriedAgRef))
	assert(t, ml.carriedFwRef[fid1] == 2, spew.Sdump(ml.carriedFwRef))
	assert(t, ml.carriedFwRef[fid2] == 1, spew.Sdump(ml.carriedFwRef))
	assert(t, ml.carriedAgRef[agid1] == 2, spew.Sdump(ml.carriedAgRef))
	assert(t, ml.carriedAgRef[agid2] == 1, spew.Sdump(ml.carriedAgRef))

	// all back, no change
	if s, err := mesos.CloneSnapshot(snap); err != nil {
		t.Fatal(err)
	} else {
		ml.update(s)
	}
	assert(t, proto.Equal(&snap, &ml.snap), fmt.Sprintf("\nexpected: %s\ngot:      %s", spew.Sdump(snap), spew.Sdump(ml.snap)))

	assert(t, len(ml.taskTimers) == 0, spew.Sdump(ml.taskTimers))
	assert(t, len(ml.carriedFwRef) == 0, spew.Sdump(ml.carriedFwRef))
	assert(t, len(ml.carriedAgRef) == 0, spew.Sdump(ml.carriedAgRef))
	assert(t, ml.carriedFwRef[fid1] == 0, spew.Sdump(ml.carriedFwRef))
	assert(t, ml.carriedFwRef[fid2] == 0, spew.Sdump(ml.carriedFwRef))
	assert(t, ml.carriedAgRef[agid1] == 0, spew.Sdump(ml.carriedAgRef))
	assert(t, ml.carriedAgRef[agid2] == 0, spew.Sdump(ml.carriedAgRef))

	// No change
	if s, err := mesos.CloneSnapshot(snap); err != nil {
		t.Fatal(err)
	} else {
		ml.update(s)
	}
	assert(t, proto.Equal(&snap, &ml.snap), fmt.Sprintf("\nexpected: %s\ngot:      %s", spew.Sdump(snap), spew.Sdump(ml.snap)))

	assert(t, len(ml.taskTimers) == 0, spew.Sdump(ml.taskTimers))
	assert(t, len(ml.carriedFwRef) == 0, spew.Sdump(ml.carriedFwRef))
	assert(t, len(ml.carriedAgRef) == 0, spew.Sdump(ml.carriedAgRef))
	assert(t, ml.carriedFwRef[fid1] == 0, spew.Sdump(ml.carriedFwRef))
	assert(t, ml.carriedFwRef[fid2] == 0, spew.Sdump(ml.carriedFwRef))
	assert(t, ml.carriedAgRef[agid1] == 0, spew.Sdump(ml.carriedAgRef))
	assert(t, ml.carriedAgRef[agid2] == 0, spew.Sdump(ml.carriedAgRef))
}

func TestSimpleUpdate(t *testing.T) {
	ml := NewMesosListener()

	tid1 := "tid1"
	fid1 := "fid1"
	agid1 := "agid1"

	tid2 := "tid2"
	fid2 := "fid2"
	agid2 := "agid2"

	tid3 := "tid3"

	tid4 := "tid4"
	fid4 := "fid4"
	agid4 := "agid4"

	tid5 := "tid5"
	fid5 := "fid5"
	agid5 := "agid5"

	tid6 := "tid6"

	spec1 := snapSpec{
		tasks: []taskSpec{
			{id: tid1, fid: fid1, agid: agid1},
			{id: tid2, fid: fid2, agid: agid2},
			{id: tid3, fid: fid1, agid: agid1},
		},
		frameworks: []frameworkSpec{
			{id: fid1},
			{id: fid2},
		},
		agents: []agentSpec{
			{id: agid1},
			{id: agid2},
		},
	}

	spec2 := snapSpec{
		tasks: []taskSpec{
			{id: tid1, fid: fid1, agid: agid1},
			{id: tid4, fid: fid4, agid: agid4},
			{id: tid5, fid: fid5, agid: agid5},
			{id: tid6, fid: fid5, agid: agid5},
		},
		frameworks: []frameworkSpec{
			{id: fid1},
			{id: fid4},
			{id: fid5},
		},
		agents: []agentSpec{
			{id: agid1},
			{id: agid4},
			{id: agid5},
		},
	}

	snap1, snap1Err := mkSnap(spec1)
	if snap1Err != nil {
		t.Fatal(snap1Err)
	}
	spew.Dump(snap1)

	snap2, snap2Err := mkSnap(spec2)
	if snap2Err != nil {
		t.Fatal(snap2Err)
	}
	spew.Dump(snap2)

	// Setup
	if s, err := mesos.CloneSnapshot(snap1); err != nil {
		t.Fatal(err)
	} else {
		ml.update(s)
	}
	assert(t, proto.Equal(&snap1, &ml.snap), fmt.Sprintf("\nexpected: %s\ngot:      %s", spew.Sdump(snap1), spew.Sdump(ml.snap)))

	// Update
	if s, err := mesos.CloneSnapshot(snap2); err != nil {
		t.Fatal(err)
	} else {
		ml.update(s)
	}
	assert(t, proto.Equal(&snap2, &ml.snap), fmt.Sprintf("\nexpected: %s\ngot:      %s", spew.Sdump(snap2), spew.Sdump(ml.snap)))
}

func mkSnap(spec snapSpec) (mesos_v1.FrameworkSnapshot, error) {
	snap := mesos_v1.FrameworkSnapshot{
		Tasks:      make(map[string]*mesos_v1.Task),
		Agents:     make(map[string]*mesos_v1.AgentInfo),
		Frameworks: make(map[string]*mesos_v1.FrameworkInfo),
	}

	for _, aSpecOrig := range spec.agents {
		// Make a copy of the built-in range item (second arg) as you can't
		// take the pointer of it since the address gets reused.

		aSpec := aSpecOrig

		if _, ok := snap.Agents[aSpec.id]; ok {
			return mesos_v1.FrameworkSnapshot{},
				fmt.Errorf("agent %s already exists", aSpec.id)
		}
		snap.Agents[aSpec.id] = &mesos_v1.AgentInfo{
			Id: &mesos_v1.AgentID{Value: &aSpec.id},
		}
	}

	for _, fSpecOrig := range spec.frameworks {
		// Make a copy of the built-in range item (second arg) as you can't
		// take the pointer of it since the address gets reused.

		fSpec := fSpecOrig

		if _, ok := snap.Frameworks[fSpec.id]; ok {
			return mesos_v1.FrameworkSnapshot{},
				fmt.Errorf("framework %s already exists", fSpec.id)
		}
		snap.Frameworks[fSpec.id] = &mesos_v1.FrameworkInfo{
			Id: &mesos_v1.FrameworkID{Value: &fSpec.id},
		}
	}

	for _, tSpecOrig := range spec.tasks {
		// Make a copy of the built-in range item (second arg) as you can't
		// take the pointer of it since the address gets reused.

		tSpec := tSpecOrig

		if _, ok := snap.Tasks[tSpec.id]; ok {
			return mesos_v1.FrameworkSnapshot{},
				fmt.Errorf("task %s already exists", tSpec.id)
		}

		if _, ok := snap.Agents[tSpec.agid]; !ok {
			return mesos_v1.FrameworkSnapshot{},
				fmt.Errorf("agent %s not found", tSpec.agid)
		}
		if _, ok := snap.Frameworks[tSpec.fid]; !ok {
			return mesos_v1.FrameworkSnapshot{},
				fmt.Errorf("framework %s not found", tSpec.fid)
		}

		snap.Tasks[tSpec.id] = &mesos_v1.Task{
			TaskId:      &mesos_v1.TaskID{Value: &tSpec.id},
			FrameworkId: &mesos_v1.FrameworkID{Value: &tSpec.fid},
			AgentId:     &mesos_v1.AgentID{Value: &tSpec.agid},
		}
	}
	return snap, nil
}

func assert(t *testing.T, b bool, s string) {
	if !b {
		t.Fatalf("%s\n%s", s, debug.Stack())
	}
}
