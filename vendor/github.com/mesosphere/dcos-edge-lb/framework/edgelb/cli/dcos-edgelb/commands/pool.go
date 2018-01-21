package commands

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"strings"

	sdkClient "github.com/mesosphere/dcos-commons/cli/client"
	edgelbOperations "github.com/mesosphere/dcos-edge-lb/apiserver/client/operations"
	edgelbDcos "github.com/mesosphere/dcos-edge-lb/apiserver/dcos"
	models "github.com/mesosphere/dcos-edge-lb/apiserver/models"
	edgelb "github.com/mesosphere/dcos-edge-lb/framework/edgelb/cli/dcos-edgelb/client"
	"github.com/olekukonko/tablewriter"
	"gopkg.in/alecthomas/kingpin.v3-unstable"
)

// PodInfoValueField is a sdk pod info field value
type PodInfoValueField struct {
	Value string `json:"value"`
}

// PodInfoStatus is the sdk pod info status
type PodInfoStatus struct {
	ContainerStatus json.RawMessage   `json:"containerStatus"`
	TaskID          PodInfoValueField `json:"taskId"`
	SlaveID         PodInfoValueField `json:"slaveId"`
	ExecutorID      PodInfoValueField `json:"executorId"`
	State           string            `json:"state"`
}

// PodInfoExecutor is the sdk pod info executor
type PodInfoExecutor struct {
	FrameworkID PodInfoValueField `json:"frameworkId"`
}

// PodInfoInfo is the sdk pod info
type PodInfoInfo struct {
	Name      string          `json:"name"`
	Discovery json.RawMessage `json:"discovery"`
	Executor  PodInfoExecutor `json:"executor"`
}

// PodInfo is the sdk pod info top level object
type PodInfo struct {
	Info   PodInfoInfo   `json:"info"`
	Status PodInfoStatus `json:"status"`
}

// EndpointsEndpoint is the sdk endpoint
type EndpointsEndpoint struct {
	Address []string `json:"address"`
	DNS     []string `json:"dns"`
}

// EndpointSummary is our representation for endpoints
type EndpointSummary struct {
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
}

// PoolHandler handles state for pool commands
type PoolHandler struct {
	sdk           sdkAPI
	printDocs     bool
	json          bool
	raw           bool
	taskIDs       bool
	convertToJSON string
	name          string
	poolFile      string
	templateFile  string
}

// HandlePoolSection handles pool commands
func HandlePoolSection(app *kingpin.Application) {
	cmd := &PoolHandler{
		sdk: newSDKAPI(),
	}

	list := app.Command("list", "List the names of all configured pools").Action(cmd.runList)
	list.Flag("json", "Show unparsed JSON response").BoolVar(&cmd.json)

	create := app.
		Command("create", "Creates a single pool given a definition file written in JSON or YAML").
		Action(cmd.runCreate)
	create.Arg("pool-file", "JSON or YAML file containing pool configuration").Required().StringVar(&cmd.poolFile)
	create.Flag("json", "Show unparsed JSON response").BoolVar(&cmd.json)

	show := app.
		Command("show", "Shows the pool definition for a given pool name. "+
			"If pool-name is omitted, all pool configurations are shown").
		Action(cmd.runShow)
	show.Arg("pool-name", "Pool name").StringVar(&cmd.name)
	show.Flag("reference", "Print the configuration reference").BoolVar(&cmd.printDocs)
	show.Flag("convert-to-json", "Converts local YAML file to JSON").StringVar(&cmd.convertToJSON)
	show.Flag("json", "Show unparsed JSON response").BoolVar(&cmd.json)

	update := app.Command("update", "Updates an existing pool").Action(cmd.runUpdate)
	update.Arg("pool-file", "JSON or YAML file containing pool configuration").Required().StringVar(&cmd.poolFile)
	update.Flag("json", "Show unparsed JSON response").BoolVar(&cmd.json)

	delete := app.Command("delete", "Deletes and uninstalls an existing pool").Action(cmd.runDelete)
	delete.Arg("pool-name", "Pool name").Required().StringVar(&cmd.name)

	status := app.
		Command("status", "List of load-balancer task information associated with "+
			"the pool such as agent IP address, task ID, etc").
		Action(cmd.runStatus)
	status.Arg("pool-name", "Pool name").Required().StringVar(&cmd.name)
	status.Flag("task-ids", "Only print the task ids").BoolVar(&cmd.taskIDs)
	status.Flag("json", "Show JSON summary response").BoolVar(&cmd.json)

	endpoints := app.
		Command("endpoints", "List of all endpoints for the pool").
		Action(cmd.runEndpoints)
	endpoints.Arg("pool-name", "Pool name").Required().StringVar(&cmd.name)
	endpoints.Flag("json", "Show unparsed JSON response").BoolVar(&cmd.json)

	lbConfig := app.
		Command("lb-config", "Shows the running load-balancer config associated with the pool").
		Action(cmd.runLBConfig)
	lbConfig.Arg("pool-name", "Pool name").Required().StringVar(&cmd.name)
	lbConfig.Flag("raw", "Show unparsed load-balancer config").BoolVar(&cmd.raw)

	template := app.Command("template", "Manage load-balancer config templates")

	templateCreate := template.
		Command("create", "Creates a custom config template for a pool of load-balancers").
		Action(cmd.runTemplateCreate)
	templateCreate.Arg("pool-name", "Pool name").Required().StringVar(&cmd.name)
	templateCreate.Arg("template-file", "Template file to store").Required().StringVar(&cmd.templateFile)

	templateShow := template.
		Command("show", "Shows the load-balancer config template for an individual pool. "+
			"If pool-name is omitted, the default template is shown").
		Action(cmd.runTemplateShow)
	templateShow.Arg("pool-name", "Pool name").StringVar(&cmd.name)

	templateUpdate := template.
		Command("update", "Updates a custom config template for a pool of load-balancers").
		Action(cmd.runTemplateUpdate)
	templateUpdate.Arg("pool-name", "Pool name").Required().StringVar(&cmd.name)
	templateUpdate.Arg("template-file", "Template file to store").Required().StringVar(&cmd.templateFile)

	templateDelete := template.
		Command("delete", "Reverts a custom config template to the default value").
		Action(cmd.runTemplateDelete)
	templateDelete.Arg("pool-name", "Pool name").Required().StringVar(&cmd.name)
}

// PrintTable prints tabular output
func PrintTable(header []string, data [][]string) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader(header)
	table.SetBorder(false)
	table.SetAutoFormatHeaders(true)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	// table.SetAutoMergeCells(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.AppendBulk(data)
	table.Render()
}

func (cmd *PoolHandler) runList(a *kingpin.Application, e *kingpin.ParseElement, c *kingpin.ParseContext) error {
	params := edgelbOperations.NewGetConfigContainerParams()
	eClient, err := edgelb.New()
	if err != nil {
		return err
	}
	resp, err := eClient.GetConfigContainer(params)
	if err != nil {
		return edgelb.PrintJSONError(nil, err)
	}
	configContainer := resp.Payload
	if cmd.json {
		allPools := []interface{}{}
		for _, poolContainer := range configContainer.Pools {
			if poolContainer.APIVersion == models.APIVersionV1 {
				allPools = append(allPools, poolContainer.V1)
			} else {
				allPools = append(allPools, poolContainer.V2)
			}
		}
		return edgelb.PrintJSON(allPools)
	}
	tableHeader := []string{"Name", "APIVersion", "Count", "Role", "Ports"}
	tableData := [][]string{}
	for _, poolContainer := range configContainer.Pools {
		// Because most of the values are the same, we will use
		// the V2 (or newest) version of the pool for display purposes
		pool, err := models.V2PoolFromContainer(poolContainer)
		if err != nil {
			return err
		}
		ports := models.V2PoolBindPortsStr(pool)
		count := strconv.Itoa(int(*pool.Count))
		role := pool.Role
		row := []string{
			poolContainer.Name,
			string(poolContainer.APIVersion),
			count,
			role,
			strings.Trim(strings.Join(ports, ", "), ", "),
		}
		tableData = append(tableData, row)
	}
	PrintTable(tableHeader, tableData)
	return nil
}

func (cmd *PoolHandler) runCreate(a *kingpin.Application, e *kingpin.ParseElement, c *kingpin.ParseContext) error {
	poolContainer, err := loadPoolContainer(cmd.poolFile)
	if err != nil {
		return err
	}
	resp, err := createPool(poolContainer)
	if err != nil {
		return edgelb.PrintJSONError(nil, err)
	}
	if cmd.json {
		return edgelb.PrintJSON(resp)
	}

	fmt.Printf("Successfully created %s. Check \"dcos edgelb show %s\" or \"dcos edgelb status %s\" for deployment status\n", poolContainer.Name, poolContainer.Name, poolContainer.Name)
	return nil
}

func (cmd *PoolHandler) runShow(a *kingpin.Application, e *kingpin.ParseElement, c *kingpin.ParseContext) error {
	if cmd.printDocs {
		b, err := reference()
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}
	if cmd.convertToJSON != "" {
		b, err := prettyLoadBytes(cmd.convertToJSON)
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}
	if cmd.name == "" {
		return fmt.Errorf("required argument '%s' not provided", "pool-name")
	}
	eClient, err := edgelb.New()
	if err != nil {
		return err
	}
	params := edgelbOperations.NewGetPoolContainerParams().
		WithName(cmd.name)
	resp, err := eClient.GetPoolContainer(params)
	if err != nil {
		return edgelb.PrintJSONError(nil, err)
	}
	if cmd.json {
		return edgelb.PrintJSON(resp)
	}
	poolContainer := resp.Payload
	// Because most of the values are the same, we will use
	// the V2 (or newest) version of the pool for display purposes
	pool, err := models.V2PoolFromContainer(poolContainer)
	if err != nil {
		return err
	}
	fmt.Println("Summary:")
	tableHeader := []string{}
	tableData := [][]string{
		{"NAME", pool.Name},
		{"APIVERSION", string(pool.APIVersion)},
		{"COUNT", strconv.Itoa(int(*pool.Count))},
		{"ROLE", pool.Role},
		{"CONSTRAINTS", *pool.Constraints},
		{"STATSPORT", strconv.Itoa(int(pool.Haproxy.Stats.BindPort))},
	}
	PrintTable(tableHeader, tableData)
	fmt.Println("")
	showFrontends(pool)
	fmt.Println("")
	showBackends(pool)
	fmt.Println("")
	showMarathonServices(pool)
	fmt.Println("")
	showMesosServices(pool)
	return nil
}

func showFrontends(pool *models.V2Pool) {
	fmt.Println("Frontends:")
	tableHeader := []string{"NAME", "PORT", "PROTOCOL"}
	tableData := [][]string{}
	for _, fe := range pool.Haproxy.Frontends {
		bindPort := strconv.Itoa(int(*fe.BindPort))
		name := fe.Name
		if name == "" {
			name = "frontend_" + fe.BindAddress + "_" + bindPort
		}
		tableData = append(tableData, []string{
			name,
			bindPort,
			string(fe.Protocol),
		})
	}
	PrintTable(tableHeader, tableData)
}

func showBackends(pool *models.V2Pool) {
	fmt.Println("Backends:")
	tableHeader := []string{"FRONTEND", "NAME", "PROTOCOL", "BALANCE"}
	tableData := [][]string{}
	for _, be := range pool.Haproxy.Backends {
		frontend := ""
		for _, fe := range pool.Haproxy.Frontends {
			feName := fe.Name
			if feName == "" {
				feName = "frontend_" + fe.BindAddress + "_" + strconv.Itoa(int(*fe.BindPort))
			}
			if frontend != "" {
				break
			}
			if be.Name == fe.LinkBackend.DefaultBackend {
				frontend = feName
				break
			}
			for _, fbe := range fe.LinkBackend.Map {
				if be.Name == fbe.Backend {
					frontend = feName
					break
				}
			}
		}
		tableData = append(tableData, []string{
			frontend,
			be.Name,
			string(be.Protocol),
			be.Balance,
		})
	}
	PrintTable(tableHeader, tableData)
}

func showMarathonServices(pool *models.V2Pool) {
	fmt.Println("Marathon Services:")
	tableHeader := []string{"BACKEND", "TYPE", "SERVICE", "CONTAINER", "PORT", "CHECK"}
	tableData := [][]string{}
	for _, be := range pool.Haproxy.Backends {
		for _, s := range be.Services {
			if marathonErr := models.V2CheckMarathonService(s.Marathon); marathonErr != nil {
				continue
			}
			serv := strings.Trim(strings.Join([]string{
				s.Marathon.ServiceID,
				s.Marathon.ServiceIDPattern,
			}, ", "), ", ")
			ct := strings.Trim(strings.Join([]string{
				s.Marathon.ContainerName,
				s.Marathon.ContainerNamePattern,
			}, ", "), ", ")
			pt := s.Endpoint.PortName
			if s.Endpoint.Port != -1 {
				pt = strconv.Itoa(int(s.Endpoint.Port))
			}
			if s.Endpoint.AllPorts {
				pt = "all"
			}
			check := "enabled"
			if !*s.Endpoint.Check.Enabled {
				check = "disabled"
			}
			tableData = append(tableData, []string{
				be.Name,
				s.Endpoint.Type,
				serv,
				ct,
				pt,
				check,
			})
		}
	}
	PrintTable(tableHeader, tableData)
}

func showMesosServices(pool *models.V2Pool) {
	fmt.Println("Mesos Services:")
	tableHeader := []string{"BACKEND", "TYPE", "FRAMEWORK", "TASK", "PORT", "CHECK"}
	tableData := [][]string{}
	for _, be := range pool.Haproxy.Backends {
		for _, s := range be.Services {
			if mesosErr := models.V2CheckMesosService(s.Mesos); mesosErr != nil {
				continue
			}
			check := "enabled"
			if !*s.Endpoint.Check.Enabled {
				check = "disabled"
			}
			fw := strings.Trim(strings.Join([]string{
				s.Mesos.FrameworkName,
				s.Mesos.FrameworkNamePattern,
				s.Mesos.FrameworkID,
				s.Mesos.FrameworkIDPattern,
			}, ", "), ", ")
			tk := strings.Trim(strings.Join([]string{
				s.Mesos.TaskName,
				s.Mesos.TaskNamePattern,
				s.Mesos.TaskID,
				s.Mesos.TaskIDPattern,
			}, ", "), ", ")
			pt := s.Endpoint.PortName
			if s.Endpoint.Port != -1 {
				pt = strconv.Itoa(int(s.Endpoint.Port))
			}
			if s.Endpoint.AllPorts {
				pt = "all"
			}
			tableData = append(tableData, []string{
				be.Name,
				s.Endpoint.Type,
				fw,
				tk,
				pt,
				check,
			})
		}
	}
	PrintTable(tableHeader, tableData)
}

func (cmd *PoolHandler) runUpdate(a *kingpin.Application, e *kingpin.ParseElement, c *kingpin.ParseContext) error {
	poolContainer, err := loadPoolContainer(cmd.poolFile)
	if err != nil {
		return err
	}
	resp, err := updatePool(poolContainer)
	if err != nil {
		return edgelb.PrintJSONError(nil, err)
	}
	if cmd.json {
		return edgelb.PrintJSON(resp)
	}

	fmt.Printf("Successfully updated %s. Check \"dcos edgelb show %s\" or \"dcos edgelb status %s\" for deployment status\n", poolContainer.Name, poolContainer.Name, poolContainer.Name)
	return nil
}

func (cmd *PoolHandler) runDelete(a *kingpin.Application, e *kingpin.ParseElement, c *kingpin.ParseContext) error {
	token := edgelb.GetToken()
	params := edgelbOperations.NewV2DeletePoolParams().
		WithName(cmd.name).
		WithToken(&token)
	eClient, err := edgelb.New()
	if err != nil {
		return err
	}
	if _, err := eClient.V2DeletePool(params); err != nil {
		return edgelb.PrintJSONError(nil, err)
	}
	fmt.Printf("Successfully deleted %s. Check the DC/OS web UI for pool uninstall status.\n", cmd.name)
	return nil
}

func (cmd *PoolHandler) runStatus(a *kingpin.Application, e *kingpin.ParseElement, c *kingpin.ParseContext) error {
	poolContainer, err := fetchPoolContainer(cmd.name)
	if err != nil {
		return err
	}
	podInfos, err := getPodInfos(cmd.sdk, *poolContainer.Namespace, cmd.name)
	if err != nil {
		return err
	}
	return showStatus(podInfos, cmd.json, cmd.taskIDs)
}

func showStatus(podInfos []PodInfo, jsonOnly, taskIDsOnly bool) error {
	if jsonOnly && !taskIDsOnly {
		b, err := json.Marshal(podInfos)
		if err != nil {
			return err
		}
		sdkClient.PrintJSONBytes(b)
		return nil
	}
	var tableHeader []string
	if taskIDsOnly {
		tableHeader = []string{}
	} else {
		tableHeader = []string{"Name", "Task ID", "State"}
	}
	tableData := [][]string{}
	for _, p := range podInfos {
		if taskIDsOnly {
			tableData = append(tableData, []string{
				p.Status.TaskID.Value,
			})
		} else {
			tableData = append(tableData, []string{
				p.Info.Name,
				p.Status.TaskID.Value,
				p.Status.State,
			})
		}
	}
	if jsonOnly && taskIDsOnly {
		var taskIDs []string
		for _, r := range tableData {
			taskIDs = append(taskIDs, r[0])
		}
		b, err := json.Marshal(taskIDs)
		if err != nil {
			return err
		}
		sdkClient.PrintJSONBytes(b)
	} else {
		PrintTable(tableHeader, tableData)
	}
	return nil
}

func (cmd *PoolHandler) runEndpoints(a *kingpin.Application, e *kingpin.ParseElement, c *kingpin.ParseContext) error {
	poolContainer, err := fetchPoolContainer(cmd.name)
	if err != nil {
		return err
	}
	appID := edgelbDcos.PoolAppID(*poolContainer.Namespace, cmd.name)
	// get all endpoint names like p80, p9090
	allB, allBErr := cmd.sdk.endpointsHandleEndpoints(appID, "")
	if allBErr != nil {
		return fmt.Errorf("could not reach the pool scheduler with name '%s/%s'.\nWas the pool recently installed, updated, or uninstalled? It may still be initializing, wait a bit and try again", *poolContainer.Namespace, cmd.name)
	}
	epNames := []string{}
	if err := json.Unmarshal(allB, &epNames); err != nil {
		return err
	} else if len(epNames) == 0 {
		return fmt.Errorf("no endpoints found")
	}
	epInfos := map[string]EndpointsEndpoint{}
	for _, ep := range epNames {
		// get endpoint info
		epB, epBErr := cmd.sdk.endpointsHandleEndpoints(appID, ep)
		if epBErr != nil {
			return epBErr
		}
		epInfo := EndpointsEndpoint{}
		if err := json.Unmarshal(epB, &epInfo); err != nil {
			return err
		}
		epInfos[ep] = epInfo
	}
	if cmd.json {
		epOutput, err := json.Marshal(epInfos)
		if err != nil {
			return err
		}
		sdkClient.PrintJSONBytes(epOutput)
		return nil
	}
	showEndpoints(epInfos)
	return nil
}

func showEndpoints(epInfos map[string]EndpointsEndpoint) {
	tableHeader := []string{"Name", "Port", "Internal IP"}
	tableData := [][]string{}
	for name, ep := range epInfos {
		ips := []string{}
		port := ""
		for _, addr := range ep.Address {
			h, p, err := net.SplitHostPort(addr)
			if err != nil {
				// We shouldn't throw away all of the output because of this
				h = addr
				p = ""
			}
			ips = append(ips, h)
			port = p
		}
		tableData = append(tableData, []string{
			name,
			port,
			strings.Trim(strings.Join(ips, ", "), ", "),
		})
	}
	PrintTable(tableHeader, tableData)
}

func (cmd *PoolHandler) runLBConfig(a *kingpin.Application, e *kingpin.ParseElement, c *kingpin.ParseContext) error {
	params := edgelbOperations.NewV2GetLBConfigParams().
		WithName(cmd.name)
	eClient, err := edgelb.New()
	if err != nil {
		return err
	}
	resp, err := eClient.V2GetLBConfig(params)
	if err != nil {
		return err
	}
	if cmd.raw {
		return printStdoutLn(resp.Payload)
	}
	return printNonemptyLines(resp.Payload)
}

func (cmd *PoolHandler) runTemplateCreate(a *kingpin.Application, e *kingpin.ParseElement, c *kingpin.ParseContext) error {
	// Currently only PUT is supported for custom templates
	return cmd.runTemplateUpdate(a, e, c)
}

func (cmd *PoolHandler) runTemplateShow(a *kingpin.Application, e *kingpin.ParseElement, c *kingpin.ParseContext) error {
	// Show default template
	if cmd.name == "" {
		params := edgelbOperations.NewV2GetDefaultLBTemplateParams()
		eClient, err := edgelb.New()
		if err != nil {
			return err
		}
		resp, err := eClient.V2GetDefaultLBTemplate(params)
		if err != nil {
			return err
		}
		return printStdoutLn(resp.Payload)
	}

	params := edgelbOperations.NewV2GetLBTemplateParams().
		WithName(cmd.name)
	eClient, err := edgelb.New()
	if err != nil {
		return err
	}
	resp, err := eClient.V2GetLBTemplate(params)
	if err != nil {
		return err
	}
	return printStdoutLn(resp.Payload)
}

func (cmd *PoolHandler) runTemplateUpdate(a *kingpin.Application, e *kingpin.ParseElement, c *kingpin.ParseContext) error {
	payload, err := ioutil.ReadFile(cmd.templateFile)
	if err != nil {
		return err
	}
	payloadStr := string(payload)

	params := edgelbOperations.NewV2UpdateLBTemplateParams().
		WithName(cmd.name).
		WithTemplate(&payloadStr)
	eClient, err := edgelb.New()
	if err != nil {
		return err
	}
	resp, err := eClient.V2UpdateLBTemplate(params)
	if err != nil {
		return err
	}
	return printStdoutLn(resp.Payload)
}

func (cmd *PoolHandler) runTemplateDelete(a *kingpin.Application, e *kingpin.ParseElement, c *kingpin.ParseContext) error {
	params := edgelbOperations.NewV2DeleteLBTemplateParams().
		WithName(cmd.name)
	eClient, err := edgelb.New()
	if err != nil {
		return err
	}
	resp, err := eClient.V2DeleteLBTemplate(params)
	if err != nil {
		return err
	}
	return printStdoutLn(resp.Payload)
}

// Pool Helpers

func createPool(poolContainer *models.PoolContainer) (interface{}, error) {
	token := edgelb.GetToken()
	eClient, err := edgelb.New()
	if err != nil {
		return nil, err
	}
	if poolContainer.APIVersion == models.APIVersionV1 {
		params := edgelbOperations.NewV1CreateLoadBalancerPoolParams().
			WithLoadBalancer(poolContainer.V1).
			WithToken(&token)
		return eClient.V1CreateLoadBalancerPool(params)
	}
	params := edgelbOperations.NewV2CreatePoolParams().
		WithPool(poolContainer.V2).
		WithToken(&token)
	return eClient.V2CreatePool(params)
}

func updatePool(poolContainer *models.PoolContainer) (interface{}, error) {
	token := edgelb.GetToken()
	eClient, err := edgelb.New()
	if err != nil {
		return nil, err
	}
	if poolContainer.APIVersion == models.APIVersionV1 {
		params := edgelbOperations.NewV1UpdateLoadBalancerPoolParams().
			WithName(poolContainer.V1.Name).
			WithLoadBalancer(poolContainer.V1).
			WithToken(&token)
		return eClient.V1UpdateLoadBalancerPool(params)
	}
	params := edgelbOperations.NewV2UpdatePoolParams().
		WithName(poolContainer.V2.Name).
		WithPool(poolContainer.V2).
		WithToken(&token)
	return eClient.V2UpdatePool(params)
}

func loadPoolContainer(path string) (*models.PoolContainer, error) {
	b, err := loadAsJSONBytes(path)
	if err != nil {
		return nil, err
	}
	// Support multi pool files for now as long as they have a single pool
	configContainer, err := models.ConfigContainerFromMixedBytes(b)
	if err == nil {
		if len(configContainer.Pools) == 1 {
			return configContainer.Pools[0], nil
		}
		if len(configContainer.Pools) > 1 {
			return nil, fmt.Errorf("only one pool may be created or updated at a time, please try again with a single pool definition")
		}
	}
	var poolContainer models.PoolContainer
	if err := poolContainer.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	if poolContainer.APIVersion == models.APIVersionV1 {
		var pool models.V1Pool
		if err := pool.UnmarshalBinary(b); err != nil {
			return nil, err
		}
		poolContainer.V1 = &pool
		return &poolContainer, nil
	}
	var pool models.V2Pool
	if err := pool.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	poolContainer.V2 = &pool
	return &poolContainer, nil
}

func fetchPoolContainer(name string) (*models.PoolContainer, error) {
	eClient, err := edgelb.New()
	if err != nil {
		return nil, err
	}
	params := edgelbOperations.NewGetPoolContainerParams().
		WithName(name)
	resp, err := eClient.GetPoolContainer(params)
	if err != nil {
		return nil, edgelb.PrintJSONError(nil, err)
	}
	return resp.Payload, nil
}

// Load-Balancer / SDK Helpers

func getPodInfos(sdk sdkAPI, namespace, name string) ([]PodInfo, error) {
	appID := edgelbDcos.PoolAppID(namespace, name)
	lbNames, _, err := getPoolLbNames(sdk, appID)
	if err != nil {
		return nil, fmt.Errorf("could not reach the pool scheduler with name '%s/%s'.\nWas the pool recently installed, updated, or uninstalled? It may still be initializing, wait a bit and try again", namespace, name)
	}
	statuses := []PodInfo{}
	for _, lbName := range lbNames {
		output, outputErr := sdk.podHandleInfo(appID, lbName)
		if outputErr != nil {
			fmt.Printf("%s\n", string(output))
			return nil, outputErr
		}
		podInfo, parseErr := parsePodInfo(output, lbName)
		if parseErr != nil {
			return nil, parseErr
		}
		statuses = append(statuses, podInfo)
	}
	return statuses, nil
}

func parsePodInfo(b []byte, name string) (PodInfo, error) {
	podInfos := []PodInfo{}
	podInfo := PodInfo{}
	if err := json.Unmarshal(b, &podInfos); err != nil {
		return podInfo, err
	}
	for _, p := range podInfos {
		if p.Info.Name == fmt.Sprintf("%s-server", name) {
			podInfo = p
		}
	}
	return podInfo, nil
}

func getPoolLbNames(sdk sdkAPI, poolAppID string) ([]string, []byte, error) {
	// Returns (lbNames, rawOutput, err)
	output, cliErr := sdk.podHandleList(poolAppID)
	if cliErr != nil {
		return nil, nil, cliErr
	}

	var lbNames []string
	if err := json.Unmarshal(output, &lbNames); err != nil {
		return nil, nil, err
	}
	return lbNames, output, nil
}
