package main

import (
	"context"
	"flag"
	"net"
	"os"
	"strings"

	"github.com/Sirupsen/logrus"
	dcos "github.com/mesosphere/dcos-edge-lb/apiserver/dcos"
	"github.com/mesosphere/dcos-edge-lb/apiserver/logging"
	"github.com/mesosphere/dcos-edge-lb/apiserver/mesos-listener/listener"
	"github.com/mesosphere/dcos-edge-lb/apiserver/mesos-listener/server"
	"github.com/mesosphere/dcos-edge-lb/apiserver/mesos-listener/util"
	"github.com/mesosphere/dcos-edge-lb/go-mesos-operator/mesos"
)

const (
	defaultDcosAddr      string = "leader.mesos"
	defaultDcosProt      string = "https"
	defaultMesosAddr     string = "leader.mesos:5050"
	defaultMesosProt     string = "https"
	defaultBindAddr      string = "127.0.0.1:3535"
	defaultLogLevel      string = "debug"
	defaultDcosAuthCreds string = ""
)

var dcosAuthCreds string
var dcosAddr string
var dcosProt string
var mesosAddr string
var mesosProt string
var bindAddr string
var logLevelStr string

var logger = util.Logger

func main() {
	logger.Formatter = logging.NewFormatter("[mesos-listener] ")
	flag.StringVar(&dcosAuthCreds, "dcosAuthCreds", defaultDcosAuthCreds, "DC/OS service account credentials")
	flag.StringVar(&dcosAddr, "dcosaddr", defaultDcosAddr, "DC/OS address")
	flag.StringVar(&dcosProt, "dcosprot", defaultDcosProt, "DC/OS protocol")
	flag.StringVar(&mesosAddr, "mesosaddr", defaultMesosAddr, "Mesos address")
	flag.StringVar(&mesosProt, "mesosprot", defaultMesosProt, "Mesos protocol")
	flag.StringVar(&bindAddr, "bindaddr", defaultBindAddr, "Bind address")
	flag.StringVar(&logLevelStr, "loglevel", defaultLogLevel, "Log level")
	flag.Parse()

	switch strings.ToLower(logLevelStr) {
	case "debug":
		logger.Level = logrus.DebugLevel
	case "info":
		logger.Level = logrus.InfoLevel
	case "warn":
		logger.Level = logrus.WarnLevel
	case "error":
		logger.Level = logrus.ErrorLevel
	default:
		logger.WithField("logLevelStr", logLevelStr).Fatal("(main) invalid log level")
	}

	logger.WithFields(logrus.Fields{
		"mesosAddr":   mesosAddr,
		"mesosProt":   mesosProt,
		"bindAddr":    bindAddr,
		"logLevelStr": logLevelStr,
	}).Info("(main) args")

	run(mesosAddr, mesosProt, bindAddr)
}

func run(mesosAddr, mesosProt, bindAddr string) {
	netlis, err := net.Listen("tcp", bindAddr)
	if err != nil {
		logger.WithError(err).Fatal("(main) failed to listen")
	}

	var prot mesos.Protocol
	switch strings.ToLower(mesosProt) {
	case "https":
		prot = mesos.HTTPS
	case "http":
		prot = mesos.HTTP
	default:
		logger.Fatal("(main) invalid mesos protocol")
	}

	if dcosAuthCreds == "" {
		dcosAuthCreds = os.Getenv("DCOS_SERVICE_ACCOUNT_CREDENTIAL")
	}

	mkClient := dcos.MakeClientFn(dcosAuthCreds, dcosAddr, dcosProt)

	logger.Info("(main) create mesos listener")
	mesoslis := listener.NewMesosListener()

	logger.Info("(main) start mesos listener")
	mesoslis.Listen(context.Background(), mesosAddr, prot, mkClient)

	logger.Info("(main) start server")
	for {
		ctx, cancel := context.WithCancel(context.Background())
		if err := server.Serve(ctx, mesoslis, netlis); err != nil {
			logger.WithError(err).Error("(main) server crashed")
			cancel()
			continue
		}
		logger.Error("(main) server terminated")
		cancel()
		break
	}
}
