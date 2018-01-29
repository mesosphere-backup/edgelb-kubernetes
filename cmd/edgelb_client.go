package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	//logging library
	"github.com/Sirupsen/logrus"

	// DC/OS dependencies
	"github.com/dcos/dcos-go/dcos/http/transport"
	"github.com/mesosphere/dcos-commons/cli/config"

	// Edge-lb dependencies
	edgelbOperations "github.com/mesosphere/dcos-edge-lb/apiserver/client/operations"
	"github.com/mesosphere/dcos-edge-lb/apiserver/dcos"
	"github.com/mesosphere/dcos-edge-lb/apiserver/util"
	edgelb "github.com/mesosphere/dcos-edge-lb/framework/edgelb/cli/dcos-edgelb/client"
)

func check(e error) {
	if e != nil {
		panic(fmt.Sprintf("Something went horribly wrong:%s", e))
	}
}

func main() {

	// Setting up the global service name and DC/OS URL.
	config.ServiceName = "edgelb"
	config.DcosURL = "https://leader.mesos"

	logger := util.Logger
	logger.SetLevel(logrus.DebugLevel)

	// Read the creds:
	dat, err := ioutil.ReadFile("/tmp/edgelb-secrets.json")
	check(err)
	fmt.Print(string(dat))

	dcosCredsStr := string(dat)

	httpClient := &http.Client{
		Transport: &http.Transport{},
	}

	// Setup HTTPS client
	tlsConfig := &tls.Config{}
	tlsConfig.InsecureSkipVerify = true
	httpClient.Transport = &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	dcosCreds := &dcos.AuthCreds{}
	if err := json.Unmarshal([]byte(dcosCredsStr), dcosCreds); err != nil {
		panic(fmt.Sprintf("Failed to decode dcos auth credentials. Error: %s", err))
	}

	creds := transport.OptionCredentials(dcosCreds.UID, dcosCreds.Secret, dcosCreds.LoginEndpoint)
	expire := transport.OptionTokenExpire(time.Minute * 10)
	rt, err := transport.NewRoundTripper(httpClient.Transport, creds, expire)
	if err != nil {
		panic(fmt.Sprintf("Failed to create HTTP client with configured service account: %s", err))
	}

	params := edgelbOperations.NewPingParams()
	edgelbClient, err := edgelb.NewWithRoundTripper(rt)
	if err != nil {
		panic(fmt.Sprintf("Failed to create edgelb client with configured service account: %s", err))
	}
	resp, err := edgelbClient.Ping(params)
	if err != nil {
		panic(fmt.Sprintf("Unable to send the ping command to edgelb with:%s", err))
	}

	fmt.Printf("%s", resp.Payload)
}
