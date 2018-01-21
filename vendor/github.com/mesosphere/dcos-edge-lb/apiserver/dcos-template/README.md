DC/OS Template
===============

Note: This is a modified version of consul-template with most consul / vault functionality removed. A mesos-listener grpc client has been added as well as several template functions in `template/funcs.go` that can be used in `.ctmpl` files to get information about running mesos tasks.

[github for consul-template]: https://github.com/hashicorp/consul-template
[godocs for consul-template]: https://godoc.org/github.com/hashicorp/consul-template

Usage
-----
### Options
For the full list of options that correspond with your release, run:

```shell
dcos-template -h
```

### Command Line
The CLI interface supports all of the options detailed above.

Query the `127.0.0.1:3535` local mesos-listener grpc instance, rendering the template on disk at `/tmp/template.ctmpl` to `/tmp/result`, running DC/OS Template as a service until stopped:

```shell
$ dcos-template \
  -mesos 127.0.0.1:3535 \
  -template "/tmp/template.ctmpl:/tmp/result"
```

Query a local mesos-listener instance, rendering the template and restarting Nginx if the template has changed, once, polling 30s if mesos-listener is unavailable:

```shell
$ dcos-template \
  -mesos 127.0.0.1:3535 \
  -template "/tmp/template.ctmpl:/var/www/nginx.conf:service nginx restart" \
  -retry 30s \
  -once
```

Query a mesos-listener instance, rendering multiple templates and commands as a service until stopped:

```shell
$ dcos-template \
  -mesos 127.0.0.1:3535 \
  -template "/tmp/nginx.ctmpl:/var/nginx/nginx.conf:service nginx restart" \
  -template "/tmp/redis.ctmpl:/var/redis/redis.conf:service redis restart" \
  -template "/tmp/haproxy.ctmpl:/var/haproxy/haproxy.conf"
```

Query a mesos-listener instance that requires authentication, dumping the templates to stdout instead of writing to disk. In this example, the second and third parameters to the `-template` option are required but ignored. The file will not be written to disk and the optional command will not be executed:

```shell
$ dcos-template \
  -mesos 127.0.0.1:3535 \
  -template "/tmp/template.ctmpl:/tmp/result:service nginx restart"
  -dry
```

Query custom command and act as a supervisor for the given child process:

```shell
$ dcos-template \
  -template "/tmp/in.ctmpl:/tmp/result" \
  -exec "/sbin/my-server"
```

For more information, please see the [Exec Mode documentation](#exec-mode).

### Configuration File(s)
The DC/OS Template configuration files are written in [HashiCorp Configuration Language (HCL)][HCL]. By proxy, this means the DC/OS Template configuration file is JSON-compatible. For more information, please see the [HCL specification][HCL].

The Configuration file syntax interface supports all of the options detailed above, unless otherwise noted in the table.

```javascript
// This is the address of the mesos-listener instance.
mesos = "127.0.0.1:3535"

// This is the signal to listen for to trigger a reload event. The default
// value is shown below. Setting this value to the empty string will cause CT
// to not listen for any reload signals.
reload_signal = "SIGHUP"

// This is the signal to listen for to trigger a core dump event. The default
// value is shown below. Setting this value to the empty string will cause CT
// to not listen for any core dump signals.
dump_signal = "SIGQUIT"

// This is the signal to listen for to trigger a graceful stop. The default
// value is shown below. Setting this value to the empty string will cause CT
// to not listen for any graceful stop signals.
kill_signal = "SIGINT"

// This is customization around the environment in which template commands are
// executed. See the "exec" block for more information on the specific
// configuration options.
env {
  // ...
}

// This is the amount of time to wait before retrying a connection to mesos-listener.
// DC/OS Template is highly fault tolerant, meaning it does not exit in the
// face of failure. Instead, it uses exponential back-off and retry functions to
// wait for the cluster to become available, as is customary in distributed
// systems.
// XXX: implement for mesos-listener client
retry = "10s"

// This is the maximum interval to allow "stale" data.
// XXX: implement for mesos-listener client
max_stale = "10m"

// This is the log level. If you find a bug in DC/OS Template, please enable
// debug logs so we can help identify the issue. This is also available as a
// command line flag.
log_level = "warn"

// This is the path to store a PID file which will contain the process ID of the
// DC/OS Template process. This is useful if you plan to send custom signals
// to the process.
pid_file = "/path/to/pid"

// This is the quiescence timers; it defines the minimum and maximum amount of
// time to wait for the cluster to reach a consistent state before rendering a
// template. This is useful to enable in systems that have a lot of flapping,
// because it will reduce the the number of times a template is rendered.
// XXX: implement for mesos-listener client
wait {
  min = "5s"
  max = "10s"
}

// This block defines the configuration for connecting to a syslog server for
// logging.
syslog {
  // This enables syslog logging. Specifying any other option also enables
  // syslog logging.
  enabled = true

  // This is the name of the syslog facility to log to.
  facility = "LOCAL5"
}

// This block defines the configuration for exec mode. Please see the exec mode
// documentation at the bottom of this README for more information on how exec
// mode operates and the caveats of this mode.
exec {
  // This is the command to exec as a child process. There can be only one
  // command per DC/OS Template process.
  command = "/usr/bin/app"

  // This is a random splay to wait before killing the command. The default
  // value is 0 (no wait), but large clusters should consider setting a splay
  // value to prevent all child processes from reloading at the same time when
  // data changes occur. When this value is set to non-zero, DC/OS Template
  // will wait a random period of time up to the splay value before reloading
  // or killing the child process. This can be used to prevent the thundering
  // herd problem on applications that do not gracefully reload.
  splay = "5s"

  env {
    // This specifies if the child process should not inherit the parent
    // process's environment. By default, the child will have full access to the
    // environment variables of the parent. Setting this to true will send only
    // the values specified in `custom_env` to the child process.
    pristine = false

    // This specifies additional custom environment variables in the form shown
    // below to inject into the child's runtime environment. If a custom
    // environment variable shares its name with a system environment variable,
    // the custom environment variable takes precedence. Even if pristine,
    // whitelist, or blacklist is specified, all values in this option
    // are given to the child process.
    custom = ["PATH=$PATH:/etc/myapp/bin"]

    // This specifies a list of environment variables to exclusively include in
    // the list of environment variables exposed to the child process. If
    // specified, only those environment variables matching the given patterns
    // are exposed to the child process. These strings are matched using Go's
    // glob function, so wildcards are permitted.
    whitelist = ["MESOS_*"]

    // This specifies a list of environment variables to exclusively prohibit in
    // the list of environment variables exposed to the child process. If
    // specified, any environment variables matching the given patterns will not
    // be exposed to the child process, even if they are whitelisted. The values
    // in this option take precedence over the values in the whitelist.
    // These strings are matched using Go's glob function, so wildcards are
    // permitted.
    blacklist = ["PLUGIN_*"]
  }

  // This defines the signal that will be sent to the child process when a
  // change occurs in a watched template. The signal will only be sent after
  // the process is started, and the process will only be started after all
  // dependent templates have been rendered at least once. The default value
  // is "" (empty or nil), which tells DC/OS Template to restart the child
  // process instead of sending it a signal. This is useful for legacy
  // applications or applications that cannot properly reload their
  // configuration without a full reload.
  reload_signal = "SIGUSR1"

  // This defines the signal sent to the child process when DC/OS Template is
  // gracefully shutting down. The application should begin a graceful cleanup.
  // If the application does not terminate before the `kill_timeout`, it will
  // be terminated (effectively "kill -9"). The default value is "SIGTERM".
  kill_signal = "SIGINT"

  // This defines the amount of time to wait for the child process to gracefully
  // terminate when DC/OS Template exits. After this specified time, the child
  // process will be force-killed (effectively "kill -9"). The default value is
  // "30s".
  kill_timeout = "2s"
}

// This block defines the configuration for a template. Unlike other blocks,
// this block may be specified multiple times to configure multiple templates.
// It is also possible to configure templates via the CLI directly.
template {
  // This is the source file on disk to use as the input template. This is often
  // called the "DC/OS Template template". This option is required if not using
  // the `contents` option.
  source = "/path/on/disk/to/template.ctmpl"

  // This is the destination path on disk where the source template will render.
  // If the parent directories do not exist, DC/OS Template will attempt to
  // create them.
  destination = "/path/on/disk/where/template/will/render.txt"

  // This option allows embedding the contents of a template in the configuration
  // file rather then supplying the `source` path to the template file. This is
  // useful for short templates. This option is mutually exclusive with the
  // `source` option.
  contents = "{{ something | toJSON }}"

  // This is the optional command to run when the template is rendered. The
  // command will only run if the resulting template changes. The command must
  // return within 30s (configurable), and it must have a successful exit code.
  // DC/OS Template is not a replacement for a process monitor or init system.
  command = "restart service foo"

  // This is the maximum amount of time to wait for the optional command to
  // return. Default is 30s.
  command_timeout = "60s"

  // This is the permission to render the file. If this option is left
  // unspecified, DC/OS Template will attempt to match the permissions of the
  // file that already exists at the destination path. If no file exists at that
  // path, the permissions are 0644.
  perms = 0600

  // This option backs up the previously rendered template at the destination
  // path before writing a new one. It keeps exactly one backup. This option is
  // useful for preventing accidental changes to the data without having a
  // rollback strategy.
  backup = true

  // These are the delimiters to use in the template. The default is "{{" and
  // "}}", but for some templates, it may be easier to use a different delimiter
  // that does not conflict with the output file itself.
  left_delimiter  = "{{"
  right_delimiter = "}}"

  // This is the `minimum(:maximum)` to wait before rendering a new template to
  // disk and triggering a command, separated by a colon (`:`). If the optional
  // maximum value is omitted, it is assumed to be 4x the required minimum value.
  // This is a numeric time with a unit suffix ("5s"). There is no default value.
  // The wait value for a template takes precedence over any globally-configured
  // wait.
  wait {
    min = "2s"
    max = "10s"
  }
}
```

Note: Not all fields are required. For example, if you are not logging to syslog, you do not need to specify a syslog configuration.

Query a mesos-listener instance, rendering the template on disk at `/tmp/template.ctmpl` to `/tmp/result`, running DC/OS Template as a service until stopped:

```javascript
mesos = "127.0.0.1:3535"

template {
  source      = "/tmp/template.ctmpl"
  destination = "/tmp/result"
}
```

If a directory is given instead of a file, all files in the directory (recursively) will be merged in [lexical order](http://golang.org/pkg/path/filepath/#Walk). So if multiple files declare a "mesos" key for instance, the last one will be used. Please note that symbolic links [are not followed](https://github.com/golang/go/issues/4759).

**Commands specified on the command line take precedence over those defined in a config file!**

### Templating Language
DC/OS Template consumes template files in the [Go Template][] format. If you are not familiar with the syntax, we recommend reading the documentation, but it is similar in appearance to Mustache, Handlebars, or Liquid.

In addition to the [Go-provided template functions][Go Template], DC/OS Template exposes the following functions:

#### API Functions

##### `file`
Read and output the contents of a local file on disk. If the file cannot be read, an error will occur. Files are read using the following syntax:

```liquid
{{file "/path/to/local/file"}}
```

This example will out the entire contents of the file at `/path/to/local/file` into the template. Note: this does not process nested templates.

- - -

#### Scratch

The scratchpad (or "scratch" for short) is available within the context of a template to store temporary data or computations. Scratch data is not shared between templates and is not cached between invocations.

All scratch functions are prefixed with an underscore.

##### `scratch.Key`

Returns a boolean if data exists in the scratchpad at the named key. Even if the
data at that key is "nil", this still returns true.

```liquid
{{ scratch.Key "foo" }}
```

##### `scratch.Get`

Returns the value in the scratchpad at the named key. If the data does not
exist, this will return "nil".

```liquid
{{ scratch.Key "foo" }}
```

##### `scratch.Set`

Saves the given value at the given key. If data already exists at that key, it
is overwritten.

```liquid
{{ scratch.Set "foo" "bar" }}
```

##### `scratch.SetX`

This behaves exactly the same as `Set`, but does not overwrite if the value
already exists.

```liquid
{{ scratch.SetX "foo" "bar" }}
```

##### `scratch.MapSet`

Saves a value in a named key in the map. If data already exists at that key, it
is overwritten.

```liquid
{{ scratch.MapSet "vars" "foo" "bar" }}
```

##### `scratch.MapSetX`

This behaves exactly the same as `MapSet`, but does not overwrite if the value
already exists.

```liquid
{{ scratch.MapSetX "vars" "foo" "bar" }}
```

##### `scratch.MapValues`

Returns a sorted list (by key) of all values in the named map.

```liquid
{{ scratch.MapValues "vars" }}
```

- - -

#### Helper Functions

##### `env`
Reads the given environment variable accessible to the current process.

```liquid
{{env "MY_VAR"}}
```

This function can be chained to manipulate the output:

```liquid
{{env "MY_VAR" | toLower}}
```

##### `executeTemplate`
Executes and returns a defined template.

```liquid
{{define "custom"}}my custom template{{end}}

This is my other template:
{{executeTemplate "custom"}}

And I can call it multiple times:
{{executeTemplate "custom"}}

Even with a new context:
{{executeTemplate "custom" 42}}

Or save it to a variable:
{{$var := executeTemplate "custom"}}
```

##### `in`
Determines if a needle is within an iterable element.

```liquid
{{ if in .Tags "production" }}
# ...
{{ end }}
```

##### `loop`
Accepts varying parameters and differs its behavior based on those parameters.

If `loop` is given one integer, it will return a goroutine that begins at zero
and loops up to but not including the given integer:

```liquid
{{range loop 5}}
# Comment{{end}}
```

If given two integers, this function will return a goroutine that begins at
the first integer and loops up to but not including the second integer:

```liquid
{{range $i := loop 5 8}}
stanza-{{$i}}{{end}}
```

which would render:

```text
stanza-5
stanza-6
stanza-7
```

Note: It is not possible to get the index and the element since the function
returns a goroutine, not a slice. In other words, the following is **not valid**:

```liquid
# Will NOT work!
{{range $i, $e := loop 5 8}}
# ...{{end}}
```

##### `join`
Takes the given list of strings as a pipe and joins them on the provided string:

```liquid
{{$items | join ","}}
```

##### `trimSpace`
Takes the provided input and trims all whitespace, tabs and newlines:
```liquid
{{ file "/etc/ec2_version"| trimSpace }}
```

##### `parseBool`
Takes the given string and parses it as a boolean:

```liquid
{{"true" | parseBool}}
```

```liquid
{{if $obj_bool | parseBool}}{{end}}
```

##### `parseFloat`
Takes the given string and parses it as a base-10 float64:

```liquid
{{"1.2" | parseFloat}}
```

##### `parseInt`
Takes the given string and parses it as a base-10 int64:

```liquid
{{"1" | parseInt}}
```

This can be combined with other helpers, for example:

```liquid
{{range $i := loop $obj | parseInt}}
# ...{{end}}
```

##### `parseJSON`
Takes the given input and parses the result as JSON:

```liquid
{{with $d := $obj | parseJSON}}{{$d.name}}{{end}}
```

Note: DC/OS Template evaluates the template multiple times, and on the first evaluation the value of the key will be empty (because no data has been loaded yet). This means that templates must guard against empty responses. For example:

```liquid
{{with $d := $obj | parseJSON}}
{{if $d}}
...
{{end}}
{{end}}
```

It just works for simple keys. But fails if you want to iterate over keys or use `index` function. Wrapping code that access object with `{{ if $d }}...{{end}}` is good enough.

Alternatively you can read data from a local JSON file:

```liquid
{{with $d := file "/path/to/local/data.json" | parseJSON}}{{$d.some_key}}{{end}}
```

##### `parseUint`
Takes the given string and parses it as a base-10 int64:

```liquid
{{"1" | parseUint}}
```

See `parseInt` for examples.

##### `plugin`
Takes the name of a plugin and optional payload and executes a DC/OS Template plugin.

```liquid
{{plugin "my-plugin"}}
```

This is most commonly combined with a JSON filter for customization:

```liquid
{{$obj | toJSON | plugin "my-plugin"}}
```

Please see the [plugins](#plugins) section for more information about plugins.

##### `regexMatch`
Takes the argument as a regular expression and will return `true` if it matches on the given string, or `false` otherwise.

```liquid
{{"foo.bar" | regexMatch "foo([.a-z]+)"}}
```

##### `regexReplaceAll`
Takes the argument as a regular expression and replaces all occurrences of the regex with the given string. As in go, you can use variables like $1 to refer to subexpressions in the replacement string.

```liquid
{{"foo.bar" | regexReplaceAll "foo([.a-z]+)" "$1"}}
```

##### `replaceAll`
Takes the argument as a string and replaces all occurrences of the given string with the given string.

```liquid
{{"foo.bar" | replaceAll "." "_"}}
```

##### `split`
Splits the given string on the provided separator:

```liquid
{{"foo\nbar\n" | split "\n"}}
```

This can be combined with chained and piped with other functions:

```liquid
{{$str | toUpper | split "\n" | join ","}}
```

##### `timestamp`
Returns the current timestamp as a string (UTC). If no arguments are given, the result is the current RFC3339 timestamp:

```liquid
{{timestamp}} // e.g. 1970-01-01T00:00:00Z
```

If the optional parameter is given, it is used to format the timestamp. The magic reference date **Mon Jan 2 15:04:05 -0700 MST 2006** can be used to format the date as required:

```liquid
{{timestamp "2006-01-02"}} // e.g. 1970-01-01
```

See Go's [time.Format()](http://golang.org/pkg/time/#Time.Format) for more information.

As a special case, if the optional parameter is `"unix"`, the unix timestamp in seconds is returned as a string.

```liquid
{{timestamp "unix"}} // e.g. 0
```

##### `toJSON`
Takes a compatible object and converts it into a JSON object.

```liquid
{{ $obj | toJSON }} // e.g. {"admin":{"port":1234},"maxconns":5,"minconns":2}
```

##### `toJSONPretty`
Takes a compatible and converts it into a pretty-printed JSON object, indented by two spaces.

```liquid
{{ $obj | toJSONPretty }}
/*
{
  "admin": {
    "port": 1234
  },
  "maxconns": 5,
  "minconns": 2,
}
*/
```

##### `toLower`
Takes the argument as a string and converts it to lowercase.

```liquid
{{ $str | toLower }}
```

See Go's [strings.ToLower()](http://golang.org/pkg/strings/#ToLower) for more information.

##### `toTitle`
Takes the argument as a string and converts it to titlecase.

```liquid
{{ $str | toTitle }}
```

See Go's [strings.Title()](http://golang.org/pkg/strings/#Title) for more information.

##### `toTOML`
Takes compatible object and converts it into a TOML object.

```liquid
{{ $obj | toTOML }}
/*
maxconns = "5"
minconns = "2"

[admin]
  port = "1134"
*/
```

##### `toUpper`
Takes the argument as a string and converts it to uppercase.

```liquid
{{ $str | toUpper}}
```

See Go's [strings.ToUpper()](http://golang.org/pkg/strings/#ToUpper) for more information.

##### `toYAML`
Takes a compatible object and converts it into a pretty-printed YAML object, indented by two spaces.

```liquid
{{ $obj | toYAML }}
/*
admin:
  port: 1234
maxconns: 5
minconns: 2
*/
```

- - -

#### Math Functions

The following functions are available on floats and integer values.

##### `add`
Returns the sum of the two values.

```liquid
{{ add 1 2 }} // 3
```

This can also be used with a pipe function.

```liquid
{{ 1 | add 2 }} // 3
```

##### `subtract`
Returns the difference of the second value from the first.

```liquid
{{ subtract 2 5 }} // 3
```

This can also be used with a pipe function.

```liquid
{{ 5 | subtract 2 }}
```

Please take careful note of the order of arguments.

##### `multiply`
Returns the product of the two values.

```liquid
{{ multiply 2 2 }} // 4
```

This can also be used with a pipe function.

```liquid
{{ 2 | multiply 2 }} // 4
```

##### `divide`
Returns the division of the second value from the first.

```liquid
{{ divide 2 10 }} // 5
```

This can also be used with a pipe function.

```liquid
{{ 10 | divide 2 }} // 5
```

Please take careful note of the order or arguments.

Plugins
-------
### Authoring Plugins
For some use cases, it may be necessary to write a plugin that offloads work to another system. This is especially useful for things that may not fit in the "standard library" of DC/OS Template, but still need to be shared across multiple instances.

DC/OS Template plugins must have the following API:

```shell
$ NAME [INPUT...]
```

- `NAME` - the name of the plugin - this is also the name of the binary, either a full path or just the program name.  It will be executed in a shell with the inherited `PATH` so e.g. the plugin `cat` will run the first executable `cat` that is found on the `PATH`.
- `INPUT` - input from the template - this will always be JSON if provided

#### Important Notes

- Plugins execute user-provided scripts and pass in potentially sensitive data. 
  Nothing is validated or protected by DC/OS Template,
  so all necessary precautions and considerations should be made by template
  authors
- Plugin output must be returned as a string on stdout. Only stdout will be
  parsed for output. Be sure to log all errors, debugging messages onto stderr
  to avoid errors when DC/OS Template returns the value.
- Always `exit 0` or DC/OS Template will assume the plugin failed to execute
- Ensure the empty input case is handled correctly (see Multi-phase execution)
- Data piped into the plugin is appended after any parameters given explicitly (eg `{{ "sample-data" | plugin "my-plugin" "some-parameter"}}` will call `my-plugin some-parameter sample-data`)

Here is a sample plugin in a few different languages that removes any JSON keys that start with an underscore and returns the JSON string:

```ruby
#! /usr/bin/env ruby
require "json"

if ARGV.empty?
  puts JSON.fast_generate({})
  Kernel.exit(0)
end

hash = JSON.parse(ARGV.first)
hash.reject! { |k, _| k.start_with?("_")  }
puts JSON.fast_generate(hash)
Kernel.exit(0)
```

```go
func main() {
  arg := []byte(os.Args[1])

  var parsed map[string]interface{}
  if err := json.Unmarshal(arg, &parsed); err != nil {
    fmt.Fprintln(os.Stderr, fmt.Sprintf("err: %s", err))
    os.Exit(1)
  }

  for k, _ := range parsed {
    if string(k[0]) == "_" {
      delete(parsed, k)
    }
  }

  result, err := json.Marshal(parsed)
  if err != nil {
    fmt.Fprintln(os.Stderr, fmt.Sprintf("err: %s", err))
    os.Exit(1)
  }

  fmt.Fprintln(os.Stdout, fmt.Sprintf("%s", result))
  os.Exit(0)
}
```

Caveats
-------
### Once Mode
In Once mode, DC/OS Template will wait for all dependencies to be rendered. If a template specifies a dependency (a request) that does not exist, once mode will wait until mesos-listener returns data for that dependency. Please note that "returned data" and "empty data" are not mutually exclusive.

### Exec Mode
DC/OS Template has the ability to maintain an arbitrary child process (similar to [envconsul](https://github.com/hashicorp/envconsul)). This mode is most beneficial when running DC/OS Template in a container or on a scheduler like [Nomad](https://www.nomadproject.io) or Kubernetes. When activated, DC/OS Template will spawn and manage the lifecycle of the child process.

This mode is best-explained through example. Consider a simple application that reads a configuration file from disk and spawns a server from that configuration.

```sh
$ dcos-template \
    -template="/tmp/config.ctmpl:/tmp/server.conf" \
    -exec="/bin/my-server -config /tmp/server.conf"
```

When DC/OS Template starts, it will pull the required dependencies and populate the `/tmp/server.conf`, which the `my-server` binary consumes. After that template is rendered completely the first time, DC/OS Template spawns and manages a child process. When any of the list templates change, DC/OS Template will send the configurable reload signal to that child process. If no reload signal is provided, DC/OS Template will kill and restart the process. Additionally, in this mode, DC/OS Template will proxy any signals it receives to the child process. This enables a scheduler to control the lifecycle of the process and also eases the friction of running inside a container.

A common point of confusion is that the command string behaves the same as the shell; it does not. In the shell, when you run `foo | bar` or `foo > bar`, that is actually running as a subprocess of your shell (bash, zsh, csh, etc.). When DC/OS Template spawns the exec process, it runs outside of your shell. This behavior is _different_ from when DC/OS Template executes the template-specific reload command. If you want the ability to pipe or redirect in the exec command, you will need to spawn the process in subshell, for example:

```javascript
exec {
  command = "$SHELL -c 'my-server > /var/log/my-server.log'"
}
```

Note that when spawning like this, most shells do not proxy signals to their child by default, so your child process will not receive the signals that DC/OS Template sends to the shell. You can avoid this by writing a tiny shell wrapper and executing that instead:

```bash
#!/usr/bin/env bash
trap "kill -TERM $child" SIGTERM

/bin/my-server -config /tmp/server.conf
child=$!
wait "$child"
```

Alternatively, you can use your shell's exec function directly, if it exists:

```bash
#!/usr/bin/env bash
exec /bin/my-server -config /tmp/server.conf > /var/log/my-server.log
```

There are some additional caveats with Exec Mode, which should be considered carefully before use:

- If the child process dies, the DC/OS Template process will also die. DC/OS Template **does not supervise the process!** This is generally the responsibility of the scheduler or init system.
- The child process must remain in the foreground. This is a requirement for DC/OS Template to manage the process and send signals.
- The exec command will only start after _all_ templates have been rendered at least once. One may have multiple templates for a single DC/OS Template process, all of which must be rendered before the process starts. Consider something like an nginx or apache configuration where both the process configuration file and individual site configuration must be written in order for the service to successfully start.
- After the child process is started, any change to any dependent template will cause the reload signal to be sent to the child process. This reload signal defaults to nil, in which DC/OS Template will not kill and respawn the child. The reload signal can be specified and customized via the CLI or configuration file.
- When DC/OS Template is stopped gracefully, it will send the configurable kill signal to the child process. The default value is SIGTERM, but it can be customized via the CLI or configuration file.
- DC/OS Template will forward all signals it receives to the child process **except** its defined `reload_signal`, `dump_signal`, and `kill_signal`. If you disable these signals, DC/OS Template will forward them to the child process.
- It is not possible to have more than one exec command (although each template can still have its own reload command).
- Individual template reload commands still fire independently of the exec command.


### Termination on Error
By default DC/OS Template is highly fault-tolerant. If mesos-listener is unreachable or a template changes, DC/OS Template will happily continue running. The only exception to this rule is if the optional `command` exits non-zero. In this case, DC/OS Template will also exit non-zero. The reason for this decision is so the user can easily configure something like Upstart or God to manage DC/OS Template as a service.

If you want DC/OS Template to continue watching for changes, even if the optional command argument fails, you can append `|| true` to your command. For example:

```shell
$ dcos-template \
  -template "in.ctmpl:out.file:service nginx restart || true"
```

In this example, even if the Nginx restart command returns non-zero, the overall function will still return an OK exit code; DC/OS Template will continue to run as a service. Additionally, if you have complex logic for restarting your service, you can intelligently choose when you want DC/OS Template to exit and when you want it to continue to watch for changes. For these types of complex scripts, we recommend using a custom sh or bash script instead of putting the logic directly in the `dcos-template` command or configuration file.

### Command Environment
The current processes environment is used when executing commands. These environment variables are exported with their current values when the command executes.

### Multi-phase Execution
DC/OS Template does an n-pass evaluation of templates, accumulating dependencies on each pass. This is required due to nested dependencies.

Because of this implementation, template functions need a default value that is an acceptable parameter to a `range` function (or similar), but does not actually execute the inner loop (which would cause a panic). This is important to mention because complex templates **must** account for the "empty" case. 

Running and Process Lifecycle
-----------------------------
While there are multiple ways to run DC/OS Template, the most common pattern is to run DC/OS Template as a system service. When DC/OS Template first starts, it reads any configuration files and templates from disk and loads them into memory. From that point forward, changes to the files on disk do not propagate to running process without a reload.

The reason for this behavior is simple and aligns with other tools like haproxy. A user may want to perform pre-flight validation checks on the configuration or templates before loading them into the process. Additionally, a user may want to update configuration and templates simultaneously. Having DC/OS Template automatically watch and reload those files on changes is both operationally dangerous and against some of the paradigms of modern infrastructure. Instead, DC/OS Template listens for the `SIGHUP` syscall to trigger a configuration reload. If you update configuration or templates, simply send `HUP` to the running DC/OS Template process and DC/OS Template will reload all the configurations and templates from disk.

Debugging
---------
DC/OS Template can print verbose debugging output. To set the log level for DC/OS Template, use the `-log-level` flag:

```shell
$ dcos-template -log-level info ...
```

You can also specify the level as debug:

```shell
$ dcos-template -log-level debug ...
```
