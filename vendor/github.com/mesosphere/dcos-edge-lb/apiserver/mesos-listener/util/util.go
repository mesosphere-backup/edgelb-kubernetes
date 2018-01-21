package util

import (
	"github.com/mesosphere/dcos-edge-lb/apiserver/logging"
)

// Logger is the custom logger, this is to avoid a bug where the global
// logrus object was bleeding into other modules that also use logrus
var Logger = logging.New()
