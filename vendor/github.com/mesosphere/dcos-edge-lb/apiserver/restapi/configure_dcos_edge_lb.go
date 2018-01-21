package restapi

import (
	"crypto/tls"
	"net/http"

	"github.com/Sirupsen/logrus"
	errors "github.com/go-openapi/errors"
	runtime "github.com/go-openapi/runtime"
	swag "github.com/go-openapi/swag"
	"github.com/mesosphere/dcos-edge-lb/apiserver/logging"
	"github.com/mesosphere/dcos-edge-lb/apiserver/restapi/operations"
	graceful "github.com/tylerb/graceful"
)

// This file is safe to edit. Once it exists it will not be overwritten

//go:generate swagger generate server --target ../.. --name  --spec ../swagger.yml

var extraFlags = struct {
	DcosAddr           string `long:"dcosAddress" description:"DC/OS address" env:"DCOS_ADDR" default:"leader.mesos"`
	DcosProt           string `long:"dcosProtocol" description:"DC/OS protocol" env:"DCOS_PROT" default:"https"`
	DcosAuthSecretName string `long:"dcosAuthSecretName" description:"DC/OS secret name" env:"DCOS_SECRET_NAME" default:""`
	DcosPrincipal      string `long:"dcosPrincipal" description:"DC/OS principal name" env:"DCOS_PRINCIPAL" default:""`
	DcosAuthCreds      string `long:"dcosAuthCreds" description:"DC/OS service account credentials" env:"DCOS_SERVICE_ACCOUNT_CREDENTIAL" default:""`
	ZkString           string `long:"zk" description:"Comma separated Zookeeper servers" env:"APIS_ZK" default:"zk-1.zk:2181,zk-2.zk:2181,zk-3.zk:2181,zk-4.zk:2181,zk-5.zk:2181"`
	ZkTimeoutStr       string `long:"zkSessionTimeout" description:"Zookeeper session timeout" env:"APIS_ZK_TIMEOUT" default:"60s"`
	ZkConfigPath       string `long:"zkPath" description:"Path in Zookeeper" env:"APIS_ZK_PATH" default:"/edgelb"`
	CfgCacheFilename   string `long:"cfgCacheFile" description:"Path to file to store config in." env:"ELB_CFGCACHE_FILENAME" default:"cfgcache.json"`
	LbWorkDir          string `long:"lbWorkDir" description:"Path where rendered configs can be found." env:"LBWORKDIR" default:"/dcosfiles"`
	DcosTemplateSv     string `long:"dcosTemplateSv" description:"DCOS template service location." env:"DCOS_TEMPLATE_SV" default:"/dcosfiles/apiserver/service/dcos-template"`
	Verbose            bool   `long:"verbose" description:"Set Verbose"`
}{}

func configureFlags(api *operations.DcosEdgeLbAPI) {
	api.CommandLineOptionsGroups = []swag.CommandLineOptionsGroup{
		{
			ShortDescription: "Zookeeper",
			LongDescription:  "Zookeeper information",
			Options:          &extraFlags,
		},
	}
}

func configureAPI(api *operations.DcosEdgeLbAPI) http.Handler {
	logger.Formatter = logging.NewFormatter("[apiserver] ")

	api.ServeError = errors.ServeError
	api.Logger = logger.Infof
	api.JSONConsumer = runtime.JSONConsumer()
	api.TxtConsumer = runtime.TextConsumer()
	api.JSONProducer = runtime.JSONProducer()
	api.TxtProducer = runtime.TextProducer()

	if extraFlags.Verbose {
		logger.Level = logrus.DebugLevel
	}

	initHandlers()

	// Common / Base Endpoints
	api.PingHandler = operations.PingHandlerFunc(PingHandler)
	api.VersionHandler = operations.VersionHandlerFunc(VersionHandler)
	api.GetConfigContainerHandler = operations.GetConfigContainerHandlerFunc(GetConfigContainerHandler)
	api.GetPoolContainerHandler = operations.GetPoolContainerHandlerFunc(GetPoolContainerHandler)

	// V2 Endpoints
	api.V2CreatePoolHandler = operations.V2CreatePoolHandlerFunc(V2CreatePoolHandler)
	api.V2DeleteLBTemplateHandler = operations.V2DeleteLBTemplateHandlerFunc(V2DeleteLBTemplateHandler)
	api.V2DeletePoolHandler = operations.V2DeletePoolHandlerFunc(V2DeletePoolHandler)
	api.V2GetConfigHandler = operations.V2GetConfigHandlerFunc(V2GetConfigHandler)
	api.V2GetDefaultLBTemplateHandler = operations.V2GetDefaultLBTemplateHandlerFunc(V2GetDefaultLBTemplateHandler)
	api.V2GetLBConfigHandler = operations.V2GetLBConfigHandlerFunc(V2GetLBConfigHandler)
	api.V2GetLBTemplateHandler = operations.V2GetLBTemplateHandlerFunc(V2GetLBTemplateHandler)
	api.V2GetPoolHandler = operations.V2GetPoolHandlerFunc(V2GetPoolHandler)
	api.V2GetPoolsHandler = operations.V2GetPoolsHandlerFunc(V2GetPoolsHandler)
	api.V2UpdateLBTemplateHandler = operations.V2UpdateLBTemplateHandlerFunc(V2UpdateLBTemplateHandler)
	api.V2UpdatePoolHandler = operations.V2UpdatePoolHandlerFunc(V2UpdatePoolHandler)

	// V1 Endpoints
	api.V1CreateLoadBalancerPoolHandler = operations.V1CreateLoadBalancerPoolHandlerFunc(V1CreateLoadBalancerPoolHandler)
	api.V1DeleteLoadBalancerArtifactHandler = operations.V1DeleteLoadBalancerArtifactHandlerFunc(V1DeleteLoadBalancerArtifactHandler)
	api.V1DeleteLoadBalancerPoolHandler = operations.V1DeleteLoadBalancerPoolHandlerFunc(V1DeleteLoadBalancerPoolHandler)
	api.V1GetConfigHandler = operations.V1GetConfigHandlerFunc(V1GetConfigHandler)
	api.V1GetLoadBalancerArtifactHandler = operations.V1GetLoadBalancerArtifactHandlerFunc(V1GetLoadBalancerArtifactHandler)
	api.V1GetLoadBalancerArtifactsHandler = operations.V1GetLoadBalancerArtifactsHandlerFunc(V1GetLoadBalancerArtifactsHandler)
	api.V1GetLoadBalancerPoolHandler = operations.V1GetLoadBalancerPoolHandlerFunc(V1GetLoadBalancerPoolHandler)
	api.V1GetLoadBalancerPoolsHandler = operations.V1GetLoadBalancerPoolsHandlerFunc(V1GetLoadBalancerPoolsHandler)
	api.V1PingHandler = operations.V1PingHandlerFunc(V1PingHandler)
	api.V1UpdateConfigHandler = operations.V1UpdateConfigHandlerFunc(V1UpdateConfigHandler)
	api.V1UpdateLoadBalancerArtifactHandler = operations.V1UpdateLoadBalancerArtifactHandlerFunc(V1UpdateLoadBalancerArtifactHandler)
	api.V1UpdateLoadBalancerPoolHandler = operations.V1UpdateLoadBalancerPoolHandlerFunc(V1UpdateLoadBalancerPoolHandler)
	api.V1VersionHandler = operations.V1VersionHandlerFunc(V1VersionHandler)

	api.ServerShutdown = ServerShutdown

	return setupGlobalMiddleware(api.Serve(setupMiddlewares))
}

// The TLS configuration before HTTPS server starts.
func configureTLS(tlsConfig *tls.Config) {
	// tlsConfig.Certificates = []Certificate
	// tlsConfig.NameToCertificate = map[string]*Certificate
	// tlsConfig.GetCertificate = func(*ClientHelloInfo) (*Certificate, error)
	// tlsConfig.GetClientCertificate = func(*CertificateRequestInfo) (*Certificate, error)
	// tlsConfig.GetConfigForClient = func(*ClientHelloInfo) (*Config, error)
	// tlsConfig.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error
	// tlsConfig.RootCAs = *x509.CertPool
	// tlsConfig.NextProtos = []string
	// tlsConfig.ServerName = string
	// tlsConfig.ClientAuth = ClientAuthType
	// tlsConfig.ClientCAs = *x509.CertPool
	// tlsConfig.InsecureSkipVerify = bool
	// tlsConfig.CipherSuites = []uint16
	// tlsConfig.PreferServerCipherSuites = bool
	// tlsConfig.SessionTicketsDisabled = bool
	// tlsConfig.SessionTicketKey = [32]byte
	// tlsConfig.ClientSessionCache = ClientSessionCache
	// tlsConfig.MinVersion = uint16
	// tlsConfig.MaxVersion = uint16
	// tlsConfig.CurvePreferences = []CurveID
	// tlsConfig.DynamicRecordSizingDisabled = bool
	// tlsConfig.Renegotiation = RenegotiationSupport
	// tlsConfig.KeyLogWriter = io.Writer
}

// As soon as server is initialized but not run yet, this function will be called.
// If you need to modify a config, store server instance to stop it individually later, this is the place.
// This function can be called multiple times, depending on the number of serving schemes.
// scheme value will be set accordingly: "http", "https" or "unix"
func configureServer(s *graceful.Server, scheme, addr string) {
	// s.Timeout = time.Duration
	// s.ListenLimit = int
	// s.TCPKeepAlive = time.Duration
	// s.ConnState = func(net.Conn, http.ConnState)
	// s.BeforeShutdown = func() bool
	// s.ShutdownInitiated = func()
	// s.NoSignalHandling = bool
	// s.Logger = *log.Logger
	// s.LogFunc = func(format string, args ...interface{})
	// s.Interrupted = bool
}

// The middleware configuration is for the handler executors. These do not apply to the swagger.json document.
// The middleware executes after routing but before authentication, binding and validation
func setupMiddlewares(handler http.Handler) http.Handler {
	return handler
}

// The middleware configuration happens before anything, this middleware also applies to serving the swagger.json document.
// So this is a good place to plug in a panic handling middleware, logging and metrics
func setupGlobalMiddleware(handler http.Handler) http.Handler {
	return handler
}
