package main

import (
	cli "github.com/mesosphere/dcos-commons/cli"
	commands "github.com/mesosphere/dcos-edge-lb/framework/edgelb/cli/dcos-edgelb/commands"
	"gopkg.in/alecthomas/kingpin.v3-unstable"
)

func main() {
	app := cli.New()
	commands.HandlePingSection(app)
	commands.HandlePoolSection(app)
	commands.HandleVersionSection(app, true)
	kingpin.MustParse(app.Parse(cli.GetArguments()))
}
