package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/Sirupsen/logrus"
	dcos "github.com/mesosphere/dcos-edge-lb/apiserver/dcos"
	mesos_v1 "github.com/mesosphere/dcos-edge-lb/go-mesos-operator/include/mesos/v1"
	"github.com/mesosphere/dcos-edge-lb/go-mesos-operator/mesos"
	"github.com/mesosphere/dcos-edge-lb/go-mesos-operator/util"
)

// THIS FILE IS PURELY FOR DEBUG PURPOSES

var logger = util.Logger

var addr string
var debug bool
var crash bool
var fullOutput bool

func main() {
	flag.StringVar(&addr, "addr", "", "mesos addr (<host>:5050)")
	flag.BoolVar(&debug, "debug", false, "debug level logging")
	flag.BoolVar(&crash, "crash", false, "randomly crash")
	flag.BoolVar(&fullOutput, "full", false, "log the full output")
	flag.Parse()

	if debug {
		logger.Level = logrus.DebugLevel
	}
	if addr == "" {
		logger.Fatal("main need addr")
	}
	dcosAuthCreds := os.Getenv("DCOS_SERVICE_ACCOUNT_CREDENTIAL")
	mkClient := dcos.MakeClientFn(dcosAuthCreds, "", "")

	for {
		logger.Info("main starting listener")
		ctx := context.Background()
		var cancel context.CancelFunc
		if crash {
			ctx, cancel = context.WithCancel(ctx)
			go func(cFunc context.CancelFunc) {
				time.Sleep(time.Second * time.Duration(rand.Int63n(10)))
				logger.Info("main [[[CRASH]]] CANCELLING CONTEXT")
				cFunc()
			}(cancel)
		}
		logger.Error(mesos.NewFrameworkListener(ctx, addr, mesos.HTTP, mkClient, handleUpdate))
		time.Sleep(time.Second)
	}
}

func handleUpdate(ctx context.Context, snapshot mesos_v1.FrameworkSnapshot, err error) {
	// Context is unused.

	if err != nil {
		logger.Fatal(err)
	}
	if fullOutput {
		logger.Info(snapshot)
		return
	}

	if snapshot.Frameworks == nil {
		logger.Error("frameworks nil")
	}
	if snapshot.Agents == nil {
		logger.Error("agents nil")
	}
	if snapshot.Tasks == nil {
		logger.Error("tasks nil")
	}

	msg := ""
	for k, v := range snapshot.Frameworks {
		msg += fmt.Sprintf("FRAMEWORK(%s, %s) ", k, v.GetName())
	}
	for k, v := range snapshot.Agents {
		msg += fmt.Sprintf("AGENT(%s, %s) ", k, v.GetHostname())
	}
	for k, v := range snapshot.Tasks {
		msg += fmt.Sprintf("TASK(%s, %s) ", k, v.GetName())
	}
	logger.Info(msg)
}
