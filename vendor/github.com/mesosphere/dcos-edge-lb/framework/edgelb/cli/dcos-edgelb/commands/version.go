package commands

import (
	"fmt"

	edgelbOperations "github.com/mesosphere/dcos-edge-lb/apiserver/client/operations"
	edgelbVersion "github.com/mesosphere/dcos-edge-lb/apiserver/version"
	edgelb "github.com/mesosphere/dcos-edge-lb/framework/edgelb/cli/dcos-edgelb/client"
	"gopkg.in/alecthomas/kingpin.v3-unstable"
)

// VersionHandler handles state for version commands
type VersionHandler struct {
	primaryCLI bool // Whether or not this is the primary (as opposed to the pool scheduler) CLI
}

// HandleVersionSection handles config commands
func HandleVersionSection(app *kingpin.Application, primaryCLI bool) {
	cmd := &VersionHandler{
		primaryCLI: primaryCLI,
	}
	app.Command("version", "Version information").Action(cmd.runVersion)
}

func (cmd *VersionHandler) runVersion(a *kingpin.Application, e *kingpin.ParseElement, c *kingpin.ParseContext) error {
	outputArr := []string{}
	errArr := []error{}

	outputArr = append(outputArr, fmt.Sprintf("client = %s", edgelbVersion.Version()))

	if cmd.primaryCLI {
		serverVersion, err := getServerVersion()
		outputArr = append(outputArr, fmt.Sprintf("server = %s", serverVersion))
		errArr = append(errArr, err)
	}

	for _, o := range outputArr {
		if err := printStdoutLn(o); err != nil {
			return err
		}
	}
	for _, err := range errArr {
		if err != nil {
			if printErr := printStderrLn(err.Error()); printErr != nil {
				return printErr
			}
		}
	}
	return nil
}

func getServerVersion() (string, error) {
	failedServerVersion := "error"

	params := edgelbOperations.NewVersionParams()
	eClient, eClientErr := edgelb.New()
	if eClientErr != nil {
		return failedServerVersion, eClientErr
	}
	resp, respErr := eClient.Version(params)
	if respErr != nil {
		return failedServerVersion, respErr
	}
	return resp.Payload, nil
}
