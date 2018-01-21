package mesos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/golang/protobuf/proto"
	dcos "github.com/mesosphere/dcos-edge-lb/apiserver/dcos"
	apisUtil "github.com/mesosphere/dcos-edge-lb/apiserver/util"
	mesos_v1 "github.com/mesosphere/dcos-edge-lb/go-mesos-operator/include/mesos/v1"
	mesos_v1_master "github.com/mesosphere/dcos-edge-lb/go-mesos-operator/include/mesos/v1/master"
	"github.com/mesosphere/dcos-edge-lb/go-mesos-operator/recordio"
	"github.com/mesosphere/dcos-edge-lb/go-mesos-operator/util"
)

var logger = util.Logger

const defaultOperatorAPI string = "api/v1"

// HTTP protocol
const HTTP Protocol = "http"

// HTTPS protocol
const HTTPS Protocol = "https"

// Protocol is the protocol to speak to Mesos on
type Protocol string

// FrameworkActionFn is the handler that consumes the snapshot
type FrameworkActionFn func(context.Context, mesos_v1.FrameworkSnapshot, error)

// Operator wraps the operator API
type Operator struct {
	addr     string             // The address of the Mesos master
	api      string             // The endpoint that serves the operator api
	mkClient func() dcos.Client // The dcos / mesos client maker
}

// FrameworkState is a representation of Mesos framework state
type FrameworkState struct {
	snap mesos_v1.FrameworkSnapshot

	// XXX Because framework updates are not sent to the subscribe stream,
	// we keep track of the number of tasks for framework deletion purposes
	fwTaskCount map[string]int // Frameworkid to number of tasks

	statemut   sync.RWMutex    // RWMutex that locks around access
	readrelay  <-chan struct{} // Reader listens for signals
	writerelay chan<- struct{} // Writer sends signals

	op    *Operator       // Operator
	opsub recordio.Parser // Operator subscribe stream
	prot  Protocol        // Protocol to speak to Mesos on
}

func relaysignal() struct{} {
	return struct{}{}
}

// NewOperator creates a new Operator
func NewOperator(addr string, mkClient func() dcos.Client) *Operator {
	return &Operator{
		addr:     addr,
		api:      defaultOperatorAPI,
		mkClient: mkClient,
	}
}

// Subscribe starts up the Mesos operator subscribe stream.
func (op *Operator) Subscribe(ctx context.Context, prot Protocol) (recordio.Parser, error) {
	// XXX Things to watch out for:
	// - Currently, mesos subscribe doesn't send updates for frameworks, this
	//   means that if a framework is launched after the initial subscribe,
	//   the update won't get here.
	// - Mesos allows the framework name to be changed on the fly. We also
	//   currently do not receive this update.

	// - Since Mesos allows the framework name to be changed on the fly,
	//   when we do a request to get_frameworks to reconcile a unknown
	//   framework_id, we should also go through and update any changed
	//   framework names.
	//   - Or should we? maybe we should just have it that the first name
	//     that is set is the one that's used forever.

	body, err := op.protoreq(ctx, prot, mesos_v1_master.Call_SUBSCRIBE)
	if err != nil {
		return nil, fmt.Errorf("operator subscribe: %s", err)
	}
	return recordio.NewParser(body), nil
}

// ParseOperatorSubscribe takes a message from the subscribe and turns it
// into the appropriate struct.
func ParseOperatorSubscribe(b []byte) (mesos_v1_master.Event, error) {
	msg := mesos_v1_master.Event{}
	if err := proto.Unmarshal(b, &msg); err != nil {
		fmterr := fmt.Errorf("failed unmarshal protobuf resp: %s", err)
		return mesos_v1_master.Event{}, fmterr
	}
	return msg, nil
}

func (op *Operator) protoreq(ctx context.Context, prot Protocol, ctype mesos_v1_master.Call_Type) (io.ReadCloser, error) {
	splitAddr := strings.SplitN(op.addr, ":", 2)
	splitHost := splitAddr[0]
	splitRest := splitAddr[1]
	resolvedAddrs, err := net.LookupHost(splitHost)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve \"%s\": %s", splitHost, err)
	}
	if len(resolvedAddrs) != 1 {
		return nil, fmt.Errorf("incorrect number of hosts for \"%s\": %v", splitHost, resolvedAddrs)
	}
	resAddr := fmt.Sprintf("%s:%s", resolvedAddrs[0], splitRest)
	logger.Debugf("resolved \"%s\" to \"%s\"", op.addr, resAddr)
	sub, err := proto.Marshal(&mesos_v1_master.Call{Type: &ctype})
	if err != nil {
		return nil, fmt.Errorf("failed protobuf marshal: %s", err)
	}

	url := fmt.Sprintf("%s://%s", prot, resAddr)
	client := op.mkClient().WithURL(url)
	mkRequest := func() (*http.Request, error) {
		req, reqErr := client.CreateRequest("POST", op.api, string(sub))
		if reqErr != nil {
			return nil, fmt.Errorf("protobuf http req failed: %s", reqErr)
		}
		req.Header.Set("Content-Type", "application/x-protobuf")
		req.Header.Set("Accept", "application/x-protobuf")
		return req, nil
	}
	resp, respErr := client.HTTPExecute(ctx, mkRequest, dcos.NOOPRetry)
	if respErr != nil {
		return nil, fmt.Errorf("protobuf http req failed: %s", respErr)
	}
	return resp.Body, nil
}

// GetFrameworks queries Mesos for all frameworks.
func (op *Operator) GetFrameworks(ctx context.Context, prot Protocol) (*mesos_v1_master.Response, error) {
	body, err := op.protoreq(ctx, prot, mesos_v1_master.Call_GET_FRAMEWORKS)
	if err != nil {
		return nil, fmt.Errorf("operator get_frameworks: %s", err)
	}

	b, err := ioutil.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("operator GetFrameworks read all: %s", err)
	}
	msg := mesos_v1_master.Response{}
	if err := proto.Unmarshal(b, &msg); err != nil {
		fmterr := fmt.Errorf("failed proto unmarshal get_frameworks resp: %s", err)
		return nil, fmterr
	}
	return &msg, nil
}

func (op *Operator) mustGetFrameworks(ctx context.Context, prot Protocol) *mesos_v1_master.Response {
	// The use case for this function should be removed. It's a dangerous
	// function. It can cause livelock.

	sleeptime := time.Second
	sleepfactor := time.Second
	maxsleep := time.Second * 10
	for {
		resp, err := op.GetFrameworks(ctx, prot)
		if err == nil {
			return resp
		}
		logger.Infof("mesos must get: %s", err)
		time.Sleep(sleeptime)
		if sleeptime < maxsleep {
			sleeptime += sleepfactor
		}
	}
}

// NewFrameworkListener listens to the Mesos subscribe stream and registers
// a callback.
func NewFrameworkListener(origCtx context.Context, addr string, prot Protocol, mkClient func() dcos.Client, consumeUpdate FrameworkActionFn) error {
	// This is the only function that should be called externally
	//
	// The reason we have select wrapping all writes to the error channel
	// is because we want the writes to be non-blocking, for example if the
	// reader of the channel dies, we don't want the goroutine hanging around
	// forever.

	// Each goroutine in this function calls cancel when it exits, that way
	// any crash will kill everything.

	ctx, cancel := context.WithCancel(origCtx)
	defer cancel()

	fs, err := newFrameworkWatcher(ctx, addr, prot, mkClient)
	if err != nil {
		return err
	}

	errC := make(chan error)
	wg := &sync.WaitGroup{}

	// Writer
	go flWriter(ctx, cancel, fs, errC, wg)

	// Reader
	go flReader(ctx, cancel, fs, errC, wg, consumeUpdate)

	err = <-errC
	wg.Wait()
	msg := fmt.Sprintf("framework listener terminated: %s", err.Error())
	return fmt.Errorf(apisUtil.MsgWithClose(msg, fs.opsub))
}

func flWriter(ctx context.Context, cancel context.CancelFunc, fs *FrameworkState, errC chan error, wg *sync.WaitGroup) {
	wg.Add(1)
	defer cancel()
	// Done should always be the very last as it signals the end of
	// the goroutine
	defer wg.Done()
	for {
		logger.Debug("mesos writer pre write")
		err := fs.writeOnce(ctx)
		logger.Debug("mesos writer post write")
		if err != nil {
			logger.WithError(err).Error("mesos writer terminated")
			select {
			case errC <- fmt.Errorf("mesos writer terminated: %s", err):
				// noop
			default:
				// noop for nonblocking
			}
			return
		}
	}
}

func flReader(ctx context.Context, cancel context.CancelFunc, fs *FrameworkState, errC chan error, wg *sync.WaitGroup, consumeUpdate FrameworkActionFn) {
	wg.Add(1)
	defer cancel()
	// Done should always be the very last as it signals the end of
	// the goroutine
	defer wg.Done()
	for {
		logger.Debug("mesos reader begin loop")
		select {
		case <-ctx.Done():
			select {
			case errC <- errors.New("reader terminated: context closed waiting for signal"):
				// noop
			default:
				// noop for nonblocking
			}
			return
		case <-fs.readrelay:
			// XXX Write an explanation on why consumeUpdate should not return
			//   an error.
			//
			//   - Shouldn't it though? I think we should change it
			//     to return an error, but will leave this not here because
			//     I forgot why I didn't think this should return an error.
			//   - One argument for why consumeUpdate should never return
			//     an error, is that since it's passed in by the caller,
			//     it makes more sense for the caller to have an error
			//     channel or something and handle the error on their own.
			logger.Debug("mesos reader received signal")
			fs.RLock()
			snap, snapErr := fs.Snap()
			fs.RUnlock()
			consumeUpdate(ctx, snap, snapErr)
		}
	}
}

func (fs *FrameworkState) writeOnce(ctx context.Context) error {
	update, err := fs.WaitUpdate(ctx)
	if err != nil {
		return fmt.Errorf("write once wait update: %s", err)
	}
	fs.Lock()
	logger.Debug("mesos write once pre update")
	changed, err := fs.Update(ctx, update)
	logger.Debug("mesos write once post update")
	if err != nil {
		return fmt.Errorf("write once update: %s", err)
	}
	fs.Unlock()
	if changed {
		logger.Debug("mesos write once changed will signal")
		select {
		case fs.writerelay <- relaysignal():
			logger.Debug("mesos write once signal update")
		default:
			// noop for non-blocking write to relay
			logger.Debug("mesos write once already signalled")
		}
	}
	return nil
}

// The recordio parser must be closed if this does not error
func newFrameworkWatcher(ctx context.Context, addr string, prot Protocol, mkClient func() dcos.Client) (*FrameworkState, error) {
	operator := NewOperator(addr, mkClient)
	reader, err := operator.Subscribe(ctx, prot)
	if err != nil {
		return nil, fmt.Errorf("init framework watcher: %s", err)
	}

	// This buffered channel is critical, if it were unbuffered then we can
	// potentially lose signals.
	relay := make(chan struct{}, 1)

	state := &FrameworkState{
		snap: mesos_v1.FrameworkSnapshot{
			Frameworks: make(map[string]*mesos_v1.FrameworkInfo),
			Tasks:      make(map[string]*mesos_v1.Task),
			Agents:     make(map[string]*mesos_v1.AgentInfo),
		},
		fwTaskCount: make(map[string]int),
		readrelay:   relay,
		writerelay:  relay,
		op:          operator,
		opsub:       reader,
		prot:        prot,
	}
	return state, nil
}

// Lock current state
func (fs *FrameworkState) Lock() {
	fs.statemut.Lock()
}

// Unlock current state
func (fs *FrameworkState) Unlock() {
	fs.statemut.Unlock()
}

// RLock current state
func (fs *FrameworkState) RLock() {
	fs.statemut.RLock()
}

// RUnlock current state
func (fs *FrameworkState) RUnlock() {
	fs.statemut.RUnlock()
}

// Snap takes a deep copy of state
func (fs *FrameworkState) Snap() (mesos_v1.FrameworkSnapshot, error) {
	// Assumes it's in a locked context

	logger.WithField("fwTaskCount", fs.fwTaskCount).Debug("taking snapshot")
	return CloneSnapshot(fs.snap)
}

// CloneSnapshot takes a deep copy of a snapshot
func CloneSnapshot(origSnap mesos_v1.FrameworkSnapshot) (mesos_v1.FrameworkSnapshot, error) {
	newSnap, ok := proto.Clone(&origSnap).(*mesos_v1.FrameworkSnapshot)
	if !ok {
		msg := "mesos clone snapshot invalid type assertion: %s"
		return mesos_v1.FrameworkSnapshot{}, fmt.Errorf(msg)
	}

	// proto.Clone() does not initialize empty maps, it leaves them nil
	if newSnap.Tasks == nil {
		newSnap.Tasks = make(map[string]*mesos_v1.Task)
	}
	if newSnap.Frameworks == nil {
		newSnap.Frameworks = make(map[string]*mesos_v1.FrameworkInfo)
	}
	if newSnap.Agents == nil {
		newSnap.Agents = make(map[string]*mesos_v1.AgentInfo)
	}

	return *newSnap, nil
}

// Update consumes new information from the stream and updates the current state.
func (fs *FrameworkState) Update(ctx context.Context, b []byte) (stateChanged bool, err error) {
	// Operates in a locked environment.

	event, err := ParseOperatorSubscribe(b)
	if err != nil {
		return false, fmt.Errorf("framework state update: %s", err)
	}

	changed := false
	switch *event.Type {
	case mesos_v1_master.Event_SUBSCRIBED:
		changed = parseSubscribeEvent(fs, event)
	case mesos_v1_master.Event_TASK_ADDED:
		changed = parseTaskAddEvent(ctx, fs, event)
	case mesos_v1_master.Event_TASK_UPDATED:
		changed = parseTaskUpdateEvent(fs, event)
	case mesos_v1_master.Event_AGENT_ADDED:
		// An agent may be re-added even if the agent was never removed, for
		// example if mesos agent restarts then it re-adds itself when coming
		// back.
		changed = parseAgentAddEvent(fs, event)
	case mesos_v1_master.Event_AGENT_REMOVED:
		changed = parseAgentRemoveEvent(fs, event)
	case mesos_v1_master.Event_UNKNOWN:
		logger.Debug("mesos unknown mesos event")
	default:
		logger.WithField("type", *event.Type).Debug("mesos unrecognized mesos event")
	}

	return changed, nil
}

func (fs *FrameworkState) addframework(finfo *mesos_v1.FrameworkInfo) {
	fid := *finfo.Id.Value
	fs.snap.Frameworks[fid] = finfo
	fs.fwTaskCount[fid] = 0
}

func (fs *FrameworkState) addtask(task *mesos_v1.Task) {
	tid := *task.TaskId.Value
	fid := *task.FrameworkId.Value
	fs.snap.Tasks[tid] = task
	fs.fwTaskCount[fid]++
}

func (fs *FrameworkState) deltask(taskid, frameworkid string) {
	fs.fwTaskCount[frameworkid]--

	// XXX Because framework updates are not sent to the subscribe stream,
	// if this task is the last one then we will consider the framework
	// deleted as well.
	if fs.fwTaskCount[frameworkid] == 0 {
		logger.WithFields(logrus.Fields{
			"frameworkid": frameworkid,
			"taskid":      taskid,
		}).Debug("framework has no tasks, deleting framework")
		delete(fs.snap.Frameworks, frameworkid)
		delete(fs.fwTaskCount, frameworkid)
	}

	delete(fs.snap.Tasks, taskid)
}

func parseSubscribeEvent(fs *FrameworkState, event mesos_v1_master.Event) (changed bool) {
	// This event only comes at the beginning of a stream, so we'll assume
	// that the state is completely empty and populate it from scratch

	for _, pbfw := range event.Subscribed.GetState.GetFrameworks.Frameworks {
		fs.addframework(pbfw.FrameworkInfo)
	}
	for _, pbtk := range event.Subscribed.GetState.GetTasks.Tasks {
		fs.addtask(pbtk)
	}
	for _, pbag := range event.Subscribed.GetState.GetAgents.Agents {
		ainfo := pbag.AgentInfo
		aid := *ainfo.Id.Value
		fs.snap.Agents[aid] = ainfo
	}
	return true
}

func parseTaskAddEvent(ctx context.Context, fs *FrameworkState, event mesos_v1_master.Event) (changed bool) {
	task := event.TaskAdded.Task

	// XXX Because framework updates are not sent to the subscribe stream,
	// if a task is added, it is possible it's linked to a framework that
	// we don't know about.
	fid := *task.FrameworkId.Value
	if _, ok := fs.snap.Frameworks[fid]; !ok {
		resp := fs.op.mustGetFrameworks(ctx, fs.prot)
		for _, fw := range resp.GetFrameworks.Frameworks {
			finfo := fw.FrameworkInfo
			if *finfo.Id.Value != fid {
				continue
			}
			fs.addframework(finfo)
			break
		}
	}

	fs.addtask(task)
	return true
}

func parseTaskUpdateEvent(fs *FrameworkState, event mesos_v1_master.Event) (changedState bool) {
	tid := *event.TaskUpdated.Status.TaskId.Value
	fs.snap.Tasks[tid].State = event.TaskUpdated.State
	fs.snap.Tasks[tid].Statuses = append(fs.snap.Tasks[tid].Statuses, event.TaskUpdated.Status)

	switch *event.TaskUpdated.State {
	case mesos_v1.TaskState_TASK_FINISHED:
		fallthrough
	case mesos_v1.TaskState_TASK_FAILED:
		fallthrough
	case mesos_v1.TaskState_TASK_LOST:
		// Even though the docs says that it may not be truly terminal, I was
		// told that if this were to come back, then it'd come back via
		// a task_add update and thus I can just kill it off here.
		fallthrough
	case mesos_v1.TaskState_TASK_KILLED:
		fallthrough
	case mesos_v1.TaskState_TASK_ERROR:
		fallthrough
	case mesos_v1.TaskState_TASK_DROPPED:
		fallthrough
	case mesos_v1.TaskState_TASK_GONE:
		fid := *event.TaskUpdated.FrameworkId.Value
		fs.deltask(tid, fid)
	}

	return true
}

func parseAgentAddEvent(fs *FrameworkState, event mesos_v1_master.Event) (changed bool) {
	ainfo := event.AgentAdded.Agent.AgentInfo
	aid := *ainfo.Id.Value
	fs.snap.Agents[aid] = ainfo
	return true
}

func parseAgentRemoveEvent(fs *FrameworkState, event mesos_v1_master.Event) (changed bool) {
	aid := *event.AgentRemoved.AgentId.Value
	delete(fs.snap.Agents, aid)
	return true
}

// WaitUpdate blocks until a new update from the stream is available.
func (fs *FrameworkState) WaitUpdate(ctx context.Context) ([]byte, error) {
	// Context is unused.

	logger.Debug("mesos wait pre update record")
	record, err := fs.opsub.Record()
	logger.Debug("mesos wait post update record")
	if err != nil {
		return nil, fmt.Errorf("error reading record: %s", err)
	}
	return record, nil
}
