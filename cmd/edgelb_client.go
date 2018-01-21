package main

import (
	"context"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/mesosphere/dcos-edge-lb/apiserver/dcos"
	"github.com/mesosphere/dcos-edge-lb/apiserver/util"
	"io/ioutil"
	"net/http"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func main() {

	logger := util.Logger
	logger.SetLevel(logrus.DebugLevel)

	// Read the creds:
	dat, err := ioutil.ReadFile("/tmp/edgelb-secrets.json")
	check(err)
	fmt.Print(string(dat))

	dcosCreds := string(dat)
	dcosAddr := "172.17.0.2/service/edge-lb/"
	dcosProt := "https"

	// Connect to Edge-lb using a `dcos client`.
	dcosClientFactory := dcos.MakeClientFn(dcosCreds, dcosAddr, dcosProt)

	if dcosClientFactory == nil {
		panic("Could not create client factory")
	}

	fmt.Printf("Created dcos client factory %v\n", dcosClientFactory)

	// Hit the echo API.
	ctx := context.Background()

	client := dcosClientFactory()

	if client == nil {
		panic("Could not create client")
	}
	client.WithURL("https://172.17.0.2/service/edgelb/")

	fmt.Printf("Created dcos client %v\n", client)

	request := func() (*http.Request, error) {
		return client.CreateRequest("GET", "/ping", "")
	}

	retry := func(resp *http.Response) bool {
		return false
	}

	fmt.Printf("Created request %v\n", request)
	_, err = client.HTTPExecute(ctx, request, retry)

	if err != nil {
		fmt.Printf("Unable to get a pong from Edge-lb %s", err)
	}

	fmt.Printf("Sent request.\n")

}
