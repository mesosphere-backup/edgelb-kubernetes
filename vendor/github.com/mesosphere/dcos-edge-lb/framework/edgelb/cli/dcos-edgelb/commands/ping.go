package commands

import (
	edgelbOperations "github.com/mesosphere/dcos-edge-lb/apiserver/client/operations"
	edgelb "github.com/mesosphere/dcos-edge-lb/framework/edgelb/cli/dcos-edgelb/client"
	"gopkg.in/alecthomas/kingpin.v3-unstable"
)

// PingHandler handles state for ping command
type PingHandler struct{}

// HandlePingSection handles ping command
func HandlePingSection(app *kingpin.Application) {
	cmd := &PingHandler{}
	app.Command("ping", "Test readiness of edgelb api server").Action(cmd.runPing)
}

func (cmd *PingHandler) runPing(a *kingpin.Application, e *kingpin.ParseElement, c *kingpin.ParseContext) error {
	params := edgelbOperations.NewPingParams()
	eClient, err := edgelb.New()
	if err != nil {
		return err
	}
	resp, err := eClient.Ping(params)
	if err != nil {
		return err
	}
	return printStdoutLn(resp.Payload)
}
