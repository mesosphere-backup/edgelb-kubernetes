package main

import (
	"github.com/mesosphere/dcos-commons/cli"
	"github.com/mesosphere/dcos-commons/cli/commands"
	edgelbCLICommands "github.com/mesosphere/dcos-edge-lb/framework/edgelb/cli/dcos-edgelb/commands"
	"gopkg.in/alecthomas/kingpin.v3-unstable"
)

func main() {
	app := cli.New()

	commands.HandleConfigSection(app)
	commands.HandleEndpointsSection(app)
	commands.HandlePlanSection(app)
	commands.HandlePodSection(app)
	commands.HandleStateSection(app)

	edgelbCLICommands.HandleVersionSection(app, false)

	kingpin.MustParse(app.Parse(cli.GetArguments()))
}
