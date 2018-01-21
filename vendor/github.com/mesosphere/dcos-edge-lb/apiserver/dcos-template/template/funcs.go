package template

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/burntsushi/toml"
	dep "github.com/mesosphere/dcos-edge-lb/apiserver/dcos-template/dependency"
	models "github.com/mesosphere/dcos-edge-lb/apiserver/models"
	mesos_v1 "github.com/mesosphere/dcos-edge-lb/go-mesos-operator/include/mesos/v1"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
)

// now is function that represents the current time in UTC. This is here
// primarily for the tests to override times.
var now = func() time.Time { return time.Now().UTC() }

type filterType string

type filterData struct {
	value string
	match filterType
}

const (
	exactFilter filterType = filterType("EXACT")
	regexFilter filterType = filterType("REGEX")
)

var validFilterTypes []filterType = []filterType{
	exactFilter,
	regexFilter,
}

func newFilterType(s string) (filterType, error) {
	fil := filterType(s)
	for _, f := range validFilterTypes {
		if f == fil {
			return fil, nil
		}
	}
	return filterType(""), fmt.Errorf("invalid filter type: %s", s)
}

func (d filterData) matches(s string) (bool, error) {
	switch d.match {
	case exactFilter:
		return d.value == s, nil
	case regexFilter:
		return regexp.Match(d.value, []byte(s))
	default:
		panic("unimplemented filter type")
	}
}

func mesosTaskFrameworkFilterFunc(b *Brain, used, missing *dep.Set) func(string, string, string, string) ([]*dep.MesosTask, error) {
	return func(framework, fmatch, task, tmatch string) ([]*dep.MesosTask, error) {
		// The way functions is tracked by the dep.<DepObj>.String() function
		// - this has to be consistent across calls or it gets killed.
		d := dep.NewMesosQuery()

		used.Add(d)

		if value, ok := b.Recall(d); ok {
			if value == nil {
				return nil, nil
			}

			fft, err := newFilterType(fmatch)
			if err != nil {
				return nil, err
			}
			tft, err := newFilterType(tmatch)
			if err != nil {
				return nil, err
			}

			fdata := filterData{
				value: framework,
				match: fft,
			}
			tdata := filterData{
				value: task,
				match: tft,
			}
			return mesosTaskFrameworkFilterHelper(value.(dep.MesosPayload).Snap, fdata, tdata)
		}

		missing.Add(d)

		return nil, nil
	}
}

// Returns tasks sorted by task id.
//
// The sorting ensures that the template is rendered the same way each
// time, which prevents consul-template from triggering a reload
// when there is no relevant state change.
func mesosTaskFrameworkFilterHelper(snap mesos_v1.FrameworkSnapshot, fdata, tdata filterData) ([]*dep.MesosTask, error) {
	taskMap := make(map[string]*dep.MesosTask)
	var tids []string

	for _, task := range snap.Tasks {
		tname := task.GetName()
		tmatches, err := tdata.matches(tname)
		if err != nil {
			return nil, err
		}
		if !tmatches {
			continue
		}

		fid := task.GetFrameworkId().GetValue()
		fwork, fworkExist := snap.Frameworks[fid]
		if !fworkExist {
			log.Printf("(funcs) mtffh task framework: framework %s for task %s not found", fid, tname)
			continue
		}

		fname := fwork.GetName()
		fmatches, err := fdata.matches(fname)
		if err != nil {
			return nil, err
		}
		if !fmatches {
			continue
		}
		if task.GetState() != mesos_v1.TaskState_TASK_RUNNING {
			log.Printf("(funcs) mtffh task state: %s not running", tname)
			continue
		}
		log.Printf("(funcs) mtffh accepting: framework %s task %s", fname, tname)
		mt := &dep.MesosTask{
			Task:  task,
			Agent: snap.Agents[task.GetAgentId().GetValue()],
		}

		tid := task.GetTaskId().GetValue()
		taskMap[tid] = mt
		tids = append(tids, tid)
	}

	sort.Strings(tids)

	var output []*dep.MesosTask
	for _, tid := range tids {
		output = append(output, taskMap[tid])
	}

	return output, nil
}

func dcosServiceFilterFunc(b *Brain, used, missing *dep.Set) func(interface{}) ([]dep.DCOSService, error) {
	return func(serviceDefinitionStruct interface{}) ([]dep.DCOSService, error) {
		serviceDefinitionBytes, err := json.Marshal(serviceDefinitionStruct)
		if err != nil {
			return nil, errors.Wrap(err, "dcosServiceFilter")
		}
		var serviceDefinition models.V2Service
		if err = serviceDefinition.UnmarshalBinary(serviceDefinitionBytes); err != nil {
			return nil, errors.Wrap(err, "dcosServiceFilter")
		}
		if serviceDefinition.Endpoint.Type == models.V2EndpointTypeADDRESS {
			return []dep.DCOSService{
				dep.DCOSService{
					Type: "address",
					Host: serviceDefinition.Endpoint.Address,
					Port: strconv.Itoa(int(serviceDefinition.Endpoint.Port)),
				},
			}, nil
		}

		// The way functions is tracked by the dep.<DepObj>.String() function
		// - this has to be consistent across calls or it gets killed.
		d := dep.NewMesosQuery()
		used.Add(d)

		value, ok := b.Recall(d)
		if !ok {
			missing.Add(d)
			return nil, nil
		}
		if value == nil {
			return nil, nil
		}
		tasks, err := getMesosTasksForService(value.(dep.MesosPayload).Snap, serviceDefinition)
		if err != nil {
			return nil, errors.Wrap(err, "dcosServiceFilter")
		}

		networkScopeLabel := "network-scope"
		nsHost := "host"
		nsContainer := "container"

		output := []dep.DCOSService{}

		for _, task := range tasks {
			if task.Task.Discovery == nil || task.Task.Discovery.Ports == nil || task.Task.Discovery.Ports.Ports == nil {
				continue
			}
			for _, port := range task.Task.Discovery.Ports.Ports {
				var portNum int32
				if serviceDefinition.Endpoint.Port > 0 {
					portNum = serviceDefinition.Endpoint.Port
				} else {
					if serviceDefinition.Endpoint.AllPorts || *port.Name == serviceDefinition.Endpoint.PortName {
						portNum = int32(*port.Number)
					} else {
						continue
					}
				}
				networkScope := nsHost
				if port.Labels != nil && port.Labels.Labels != nil {
					for _, label := range port.Labels.Labels {
						if *label.Key == networkScopeLabel {
							networkScope = *label.Value
						}
					}
				}
				if networkScope == nsHost || serviceDefinition.Endpoint.Type == models.V2EndpointTypeAGENTIP {
					output = append(output, dep.DCOSService{
						Type: "agentip",
						Host: *task.Agent.Hostname,
						Port: strconv.Itoa(int(portNum)),
					})
				} else if networkScope == nsContainer || serviceDefinition.Endpoint.Type == models.V2EndpointTypeCONTAINERIP {
					latestTaskStatusIndex := len(task.Task.Statuses) - 1
					latestTaskStatus := task.Task.Statuses[latestTaskStatusIndex]
					for _, networkInfo := range latestTaskStatus.ContainerStatus.NetworkInfos {
						for _, ipAddress := range networkInfo.IpAddresses {
							output = append(output, dep.DCOSService{
								Type: "containerip",
								Host: *ipAddress.IpAddress,
								Port: strconv.Itoa(int(portNum)),
							})
						}
					}
				}
			}
		}
		return output, nil
	}
}

// Returns tasks sorted by task id.
//
// The sorting ensures that the template is rendered the same way each
// time, which prevents consul-template from triggering a reload
// when there is no relevant state change.
func getMesosTasksForService(snap mesos_v1.FrameworkSnapshot, svc models.V2Service) ([]*dep.MesosTask, error) {
	taskMap := make(map[string]*dep.MesosTask)
	var tids []string

	// Default framework name to marathon for marathon based services
	svcFrameworkName := ""
	if svc.Marathon.ServiceID != "" || svc.Marathon.ServiceIDPattern != "" {
		svcFrameworkName = "marathon"
	} else {
		svcFrameworkName = svc.Mesos.FrameworkName
	}

	for _, task := range snap.Tasks {
		tid := task.GetTaskId().GetValue()
		tname := task.GetName()
		sID, cName := models.MesosTaskIDToMarathonServiceIDContainerName(tid)

		// Attempt to match on serviceID and containerName, continue if not
		if attempt, matches, err := matchesExactOrPattern(
			"serviceID",
			sID,
			svc.Marathon.ServiceID,
			svc.Marathon.ServiceIDPattern); attempt && !matches && err == nil {
			continue
		} else if err != nil {
			return nil, err
		}
		if attempt, matches, err := matchesExactOrPattern(
			"containerName",
			cName,
			svc.Marathon.ContainerName,
			svc.Marathon.ContainerNamePattern); attempt && !matches && err == nil {
			continue
		} else if err != nil {
			return nil, err
		}

		// Attempt to match on task name and ID, continue if not
		if attempt, matches, err := matchesExactOrPattern(
			"taskName",
			tname,
			svc.Mesos.TaskName,
			svc.Mesos.TaskNamePattern); attempt && !matches && err == nil {
			continue
		} else if err != nil {
			return nil, err
		}
		if attempt, matches, err := matchesExactOrPattern(
			"taskID",
			tid,
			svc.Mesos.TaskID,
			svc.Mesos.TaskIDPattern); attempt && !matches && err == nil {
			continue
		} else if err != nil {
			return nil, err
		}

		fid := task.GetFrameworkId().GetValue()
		fwork, fworkExist := snap.Frameworks[fid]
		if !fworkExist {
			log.Printf("(funcs) getMesosTasksForService task framework: framework %s for task %s not found", fid, tname)
			continue
		}
		fname := fwork.GetName()
		// Attempt to match on framework name and ID, continue if not
		if attempt, matches, err := matchesExactOrPattern(
			"frameworkName",
			fname,
			svcFrameworkName,
			svc.Mesos.FrameworkNamePattern); attempt && !matches && err == nil {
			continue
		} else if err != nil {
			return nil, err
		}
		if attempt, matches, err := matchesExactOrPattern(
			"frameworkID",
			fid,
			svc.Mesos.FrameworkID,
			svc.Mesos.FrameworkIDPattern); attempt && !matches && err == nil {
			continue
		} else if err != nil {
			return nil, err
		}

		// Check if the task is running
		if task.GetState() != mesos_v1.TaskState_TASK_RUNNING {
			log.Printf("(funcs) getMesosTasksForService task state: %s not running", tname)
			continue
		}
		log.Printf("(funcs) getMesosTasksForService accepting: framework %s, %s task %s, %s", fname, fid, tname, tid)
		mt := &dep.MesosTask{
			Task:  task,
			Agent: snap.Agents[task.GetAgentId().GetValue()],
		}
		taskMap[tid] = mt
		tids = append(tids, tid)
	}

	sort.Strings(tids)

	var output []*dep.MesosTask
	for _, tid := range tids {
		output = append(output, taskMap[tid])
	}

	return output, nil
}

// Returns whether or not a match was attempted, the match value, and error
func matchesExactOrPattern(name, value, exact, pattern string) (bool, bool, error) {
	if pattern != "" {
		matches, err := regexp.Match(value, []byte(pattern))
		return true, matches, err
	}
	if exact != "" {
		matches := value == exact
		return true, matches, nil
	}
	return false, false, nil
}

// executeTemplateFunc executes the given template in the context of the
// parent. If an argument is specified, it will be used as the context instead.
// This can be used for nested template definitions.
func executeTemplateFunc(t *template.Template) func(string, ...interface{}) (string, error) {
	return func(s string, data ...interface{}) (string, error) {
		var dot interface{}
		switch len(data) {
		case 0:
			dot = nil
		case 1:
			dot = data[0]
		default:
			return "", fmt.Errorf("executeTemplate: wrong number of arguments, expected 1 or 2"+
				", but got %d", len(data)+1)
		}
		var b bytes.Buffer
		if err := t.ExecuteTemplate(&b, s, dot); err != nil {
			return "", err
		}
		return b.String(), nil
	}
}

// fileFunc returns or accumulates file dependencies.
func fileFunc(b *Brain, used, missing *dep.Set) func(string) (string, error) {
	return func(s string) (string, error) {
		if len(s) == 0 {
			return "", nil
		}

		d, err := dep.NewFileQuery(s)
		if err != nil {
			return "", err
		}

		used.Add(d)

		if value, ok := b.Recall(d); ok {
			if value == nil {
				return "", nil
			}
			return value.(string), nil
		}

		missing.Add(d)

		return "", nil
	}
}

// base64Encode encodes the given value into a string represented as base64.
//
// XXX Backported from newer consul-template
func base64Encode(s string) (string, error) {
	return base64.StdEncoding.EncodeToString([]byte(s)), nil
}

// contains is a function that have reverse arguments of "in" and is designed to
// be used as a pipe instead of a function:
//
// 		{{ l | contains "thing" }}
//
func contains(v, l interface{}) (bool, error) {
	return in(l, v)
}

// containsSomeFunc returns functions to implement each of the following:
//
// 1. containsAll    - true if (∀x ∈ v then x ∈ l); false otherwise
// 2. containsAny    - true if (∃x ∈ v such that x ∈ l); false otherwise
// 3. containsNone   - true if (∀x ∈ v then x ∉ l); false otherwise
// 2. containsNotall - true if (∃x ∈ v such that x ∉ l); false otherwise
//
// ret_true - return true at end of loop for none/all; false for any/notall
// invert   - invert block test for all/notall
func containsSomeFunc(ret_true, invert bool) func([]interface{}, interface{}) (bool, error) {
	return func(v []interface{}, l interface{}) (bool, error) {
		for i := 0; i < len(v); i++ {
			if ok, _ := in(l, v[i]); ok != invert {
				return !ret_true, nil
			}
		}
		return ret_true, nil
	}
}

// envFunc returns a function which checks the value of an environment variable.
// Invokers can specify their own environment, which takes precedences over any
// real environment variables
func envFunc(env []string) func(string) (string, error) {
	return func(s string) (string, error) {
		for _, e := range env {
			split := strings.SplitN(e, "=", 2)
			k, v := split[0], split[1]
			if k == s {
				return v, nil
			}
		}
		return os.Getenv(s), nil
	}
}

// in searches for a given value in a given interface.
func in(l, v interface{}) (bool, error) {
	lv := reflect.ValueOf(l)
	vv := reflect.ValueOf(v)

	switch lv.Kind() {
	case reflect.Array, reflect.Slice:
		// if the slice contains 'interface' elements, then the element needs to be extracted directly to examine its type,
		// otherwise it will just resolve to 'interface'.
		var interfaceSlice []interface{}
		if reflect.TypeOf(l).Elem().Kind() == reflect.Interface {
			interfaceSlice = l.([]interface{})
		}

		for i := 0; i < lv.Len(); i++ {
			var lvv reflect.Value
			if interfaceSlice != nil {
				lvv = reflect.ValueOf(interfaceSlice[i])
			} else {
				lvv = lv.Index(i)
			}

			switch lvv.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				switch vv.Kind() {
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					if vv.Int() == lvv.Int() {
						return true, nil
					}
				}
			case reflect.Float32, reflect.Float64:
				switch vv.Kind() {
				case reflect.Float32, reflect.Float64:
					if vv.Float() == lvv.Float() {
						return true, nil
					}
				}
			case reflect.String:
				if vv.Type() == lvv.Type() && vv.String() == lvv.String() {
					return true, nil
				}
			}
		}
	case reflect.String:
		if vv.Type() == lv.Type() && strings.Contains(lv.String(), vv.String()) {
			return true, nil
		}
	}

	return false, nil
}

// loop accepts varying parameters and differs its behavior. If given one
// parameter, loop will return a goroutine that begins at 0 and loops until the
// given int, increasing the index by 1 each iteration. If given two parameters,
// loop will return a goroutine that begins at the first parameter and loops
// up to but not including the second parameter.
//
//    // Prints 0 1 2 3 4
// 		for _, i := range loop(5) {
// 			print(i)
// 		}
//
//    // Prints 5 6 7
// 		for _, i := range loop(5, 8) {
// 			print(i)
// 		}
//
func loop(ints ...int64) (<-chan int64, error) {
	var start, stop int64
	switch len(ints) {
	case 1:
		start, stop = 0, ints[0]
	case 2:
		start, stop = ints[0], ints[1]
	default:
		return nil, fmt.Errorf("loop: wrong number of arguments, expected 1 or 2"+
			", but got %d", len(ints))
	}

	ch := make(chan int64)

	go func() {
		for i := start; i < stop; i++ {
			ch <- i
		}
		close(ch)
	}()

	return ch, nil
}

// join is a version of strings.Join that can be piped
func join(sep string, a []string) (string, error) {
	return strings.Join(a, sep), nil
}

// TrimSpace is a version of strings.TrimSpace that can be piped
func trimSpace(s string) (string, error) {
	return strings.TrimSpace(s), nil
}

// parseBool parses a string into a boolean
func parseBool(s string) (bool, error) {
	if s == "" {
		return false, nil
	}

	result, err := strconv.ParseBool(s)
	if err != nil {
		return false, errors.Wrap(err, "parseBool")
	}
	return result, nil
}

// parseFloat parses a string into a base 10 float
func parseFloat(s string) (float64, error) {
	if s == "" {
		return 0.0, nil
	}

	result, err := strconv.ParseFloat(s, 10)
	if err != nil {
		return 0, errors.Wrap(err, "parseFloat")
	}
	return result, nil
}

// parseInt parses a string into a base 10 int
func parseInt(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}

	result, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "parseInt")
	}
	return result, nil
}

// parseJSON returns a structure for valid JSON
func parseJSON(s string) (interface{}, error) {
	if s == "" {
		return map[string]interface{}{}, nil
	}

	var data interface{}
	if err := json.Unmarshal([]byte(s), &data); err != nil {
		return nil, err
	}
	return data, nil
}

// parseUint parses a string into a base 10 int
func parseUint(s string) (uint64, error) {
	if s == "" {
		return 0, nil
	}

	result, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "parseUint")
	}
	return result, nil
}

// plugin executes a subprocess as the given command string. It is assumed the
// resulting command returns JSON which is then parsed and returned as the
// value for use in the template.
func plugin(name string, args ...string) (string, error) {
	if name == "" {
		return "", nil
	}

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)

	// Strip and trim each arg or else some plugins get confused with the newline
	// characters
	jsons := make([]string, 0, len(args))
	for _, arg := range args {
		if v := strings.TrimSpace(arg); v != "" {
			jsons = append(jsons, v)
		}
	}

	cmd := exec.Command(name, jsons...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("exec %q: %s\n\nstdout:\n\n%s\n\nstderr:\n\n%s",
			name, err, stdout.Bytes(), stderr.Bytes())
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-time.After(5 * time.Second):
		if cmd.Process != nil {
			if err := cmd.Process.Kill(); err != nil {
				return "", fmt.Errorf("exec %q: failed to kill", name)
			}
		}
		<-done // Allow the goroutine to exit
		return "", fmt.Errorf("exec %q: did not finish", name)
	case err := <-done:
		if err != nil {
			return "", fmt.Errorf("exec %q: %s\n\nstdout:\n\n%s\n\nstderr:\n\n%s",
				name, err, stdout.Bytes(), stderr.Bytes())
		}
	}

	return strings.TrimSpace(stdout.String()), nil
}

// replaceAll replaces all occurrences of a value in a string with the given
// replacement value.
func replaceAll(f, t, s string) (string, error) {
	return strings.Replace(s, f, t, -1), nil
}

// regexReplaceAll replaces all occurrences of a regular expression with
// the given replacement value.
func regexReplaceAll(re, pl, s string) (string, error) {
	compiled, err := regexp.Compile(re)
	if err != nil {
		return "", err
	}
	return compiled.ReplaceAllString(s, pl), nil
}

// regexMatch returns true or false if the string matches
// the given regular expression
func regexMatch(re, s string) (bool, error) {
	compiled, err := regexp.Compile(re)
	if err != nil {
		return false, err
	}
	return compiled.MatchString(s), nil
}

// split is a version of strings.Split that can be piped
func split(sep, s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return []string{}, nil
	}
	return strings.Split(s, sep), nil
}

// timestamp returns the current UNIX timestamp in UTC. If an argument is
// specified, it will be used to format the timestamp.
func timestamp(s ...string) (string, error) {
	switch len(s) {
	case 0:
		return now().Format(time.RFC3339), nil
	case 1:
		if s[0] == "unix" {
			return strconv.FormatInt(now().Unix(), 10), nil
		}
		return now().Format(s[0]), nil
	default:
		return "", fmt.Errorf("timestamp: wrong number of arguments, expected 0 or 1"+
			", but got %d", len(s))
	}
}

// toLower converts the given string (usually by a pipe) to lowercase.
func toLower(s string) (string, error) {
	return strings.ToLower(s), nil
}

// toJSON converts the given structure into a deeply nested JSON string.
func toJSON(i interface{}) (string, error) {
	result, err := json.Marshal(i)
	if err != nil {
		return "", errors.Wrap(err, "toJSON")
	}
	return string(bytes.TrimSpace(result)), err
}

// toJSONPretty converts the given structure into a deeply nested pretty JSON
// string.
func toJSONPretty(m map[string]interface{}) (string, error) {
	result, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", errors.Wrap(err, "toJSONPretty")
	}
	return string(bytes.TrimSpace(result)), err
}

// toTitle converts the given string (usually by a pipe) to titlecase.
func toTitle(s string) (string, error) {
	return strings.Title(s), nil
}

// toUpper converts the given string (usually by a pipe) to uppercase.
func toUpper(s string) (string, error) {
	return strings.ToUpper(s), nil
}

// toYAML converts the given structure into a deeply nested YAML string.
func toYAML(m map[string]interface{}) (string, error) {
	result, err := yaml.Marshal(m)
	if err != nil {
		return "", errors.Wrap(err, "toYAML")
	}
	return string(bytes.TrimSpace(result)), nil
}

// toTOML converts the given structure into a deeply nested TOML string.
func toTOML(m map[string]interface{}) (string, error) {
	buf := bytes.NewBuffer([]byte{})
	enc := toml.NewEncoder(buf)
	if err := enc.Encode(m); err != nil {
		return "", errors.Wrap(err, "toTOML")
	}
	result, err := ioutil.ReadAll(buf)
	if err != nil {
		return "", errors.Wrap(err, "toTOML")
	}
	return string(bytes.TrimSpace(result)), nil
}

// add returns the sum of a and b.
func add(b, a interface{}) (interface{}, error) {
	av := reflect.ValueOf(a)
	bv := reflect.ValueOf(b)

	switch av.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch bv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return av.Int() + bv.Int(), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return av.Int() + int64(bv.Uint()), nil
		case reflect.Float32, reflect.Float64:
			return float64(av.Int()) + bv.Float(), nil
		default:
			return nil, fmt.Errorf("add: unknown type for %q (%T)", bv, b)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		switch bv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return int64(av.Uint()) + bv.Int(), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return av.Uint() + bv.Uint(), nil
		case reflect.Float32, reflect.Float64:
			return float64(av.Uint()) + bv.Float(), nil
		default:
			return nil, fmt.Errorf("add: unknown type for %q (%T)", bv, b)
		}
	case reflect.Float32, reflect.Float64:
		switch bv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return av.Float() + float64(bv.Int()), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return av.Float() + float64(bv.Uint()), nil
		case reflect.Float32, reflect.Float64:
			return av.Float() + bv.Float(), nil
		default:
			return nil, fmt.Errorf("add: unknown type for %q (%T)", bv, b)
		}
	default:
		return nil, fmt.Errorf("add: unknown type for %q (%T)", av, a)
	}
}

// subtract returns the difference of b from a.
func subtract(b, a interface{}) (interface{}, error) {
	av := reflect.ValueOf(a)
	bv := reflect.ValueOf(b)

	switch av.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch bv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return av.Int() - bv.Int(), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return av.Int() - int64(bv.Uint()), nil
		case reflect.Float32, reflect.Float64:
			return float64(av.Int()) - bv.Float(), nil
		default:
			return nil, fmt.Errorf("subtract: unknown type for %q (%T)", bv, b)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		switch bv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return int64(av.Uint()) - bv.Int(), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return av.Uint() - bv.Uint(), nil
		case reflect.Float32, reflect.Float64:
			return float64(av.Uint()) - bv.Float(), nil
		default:
			return nil, fmt.Errorf("subtract: unknown type for %q (%T)", bv, b)
		}
	case reflect.Float32, reflect.Float64:
		switch bv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return av.Float() - float64(bv.Int()), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return av.Float() - float64(bv.Uint()), nil
		case reflect.Float32, reflect.Float64:
			return av.Float() - bv.Float(), nil
		default:
			return nil, fmt.Errorf("subtract: unknown type for %q (%T)", bv, b)
		}
	default:
		return nil, fmt.Errorf("subtract: unknown type for %q (%T)", av, a)
	}
}

// multiply returns the product of a and b.
func multiply(b, a interface{}) (interface{}, error) {
	av := reflect.ValueOf(a)
	bv := reflect.ValueOf(b)

	switch av.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch bv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return av.Int() * bv.Int(), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return av.Int() * int64(bv.Uint()), nil
		case reflect.Float32, reflect.Float64:
			return float64(av.Int()) * bv.Float(), nil
		default:
			return nil, fmt.Errorf("multiply: unknown type for %q (%T)", bv, b)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		switch bv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return int64(av.Uint()) * bv.Int(), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return av.Uint() * bv.Uint(), nil
		case reflect.Float32, reflect.Float64:
			return float64(av.Uint()) * bv.Float(), nil
		default:
			return nil, fmt.Errorf("multiply: unknown type for %q (%T)", bv, b)
		}
	case reflect.Float32, reflect.Float64:
		switch bv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return av.Float() * float64(bv.Int()), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return av.Float() * float64(bv.Uint()), nil
		case reflect.Float32, reflect.Float64:
			return av.Float() * bv.Float(), nil
		default:
			return nil, fmt.Errorf("multiply: unknown type for %q (%T)", bv, b)
		}
	default:
		return nil, fmt.Errorf("multiply: unknown type for %q (%T)", av, a)
	}
}

// divide returns the division of b from a.
func divide(b, a interface{}) (interface{}, error) {
	av := reflect.ValueOf(a)
	bv := reflect.ValueOf(b)

	switch av.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch bv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return av.Int() / bv.Int(), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return av.Int() / int64(bv.Uint()), nil
		case reflect.Float32, reflect.Float64:
			return float64(av.Int()) / bv.Float(), nil
		default:
			return nil, fmt.Errorf("divide: unknown type for %q (%T)", bv, b)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		switch bv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return int64(av.Uint()) / bv.Int(), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return av.Uint() / bv.Uint(), nil
		case reflect.Float32, reflect.Float64:
			return float64(av.Uint()) / bv.Float(), nil
		default:
			return nil, fmt.Errorf("divide: unknown type for %q (%T)", bv, b)
		}
	case reflect.Float32, reflect.Float64:
		switch bv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return av.Float() / float64(bv.Int()), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return av.Float() / float64(bv.Uint()), nil
		case reflect.Float32, reflect.Float64:
			return av.Float() / bv.Float(), nil
		default:
			return nil, fmt.Errorf("divide: unknown type for %q (%T)", bv, b)
		}
	default:
		return nil, fmt.Errorf("divide: unknown type for %q (%T)", av, a)
	}
}
