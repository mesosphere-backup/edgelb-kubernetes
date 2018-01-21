[![Build Status](https://jenkins.mesosphere.com/service/jenkins/buildStatus/icon?job=edge-lb-master)](https://jenkins.mesosphere.com/service/jenkins/job/edge-lb-master)

# DC/OS Edge Load Balancer

Developer information here: [DEVELOPMENT.md](DEVELOPMENT.md)

# Installation

## Add Repos

```
dcos package repo add --index=0 edgelb-aws \
  https://edge-lb-infinity-artifacts.s3.amazonaws.com/autodelete7d/master/edgelb/stub-universe-edgelb.json
dcos package repo add --index=0 edgelb-pool-aws \
  https://edge-lb-infinity-artifacts.s3.amazonaws.com/autodelete7d/master/edgelb-pool/stub-universe-edgelb-pool.json
```

## Enterprise Install

### Create a Service Account

```
dcos package install dcos-enterprise-cli --yes --cli
dcos security org service-accounts keypair edgelb-private-key.pem edgelb-public-key.pem
dcos security org service-accounts create -p edgelb-public-key.pem -d "edgelb service account" edgelb-principal
dcos security org groups add_user superusers edgelb-principal
dcos security secrets create-sa-secret --strict edgelb-private-key.pem edgelb-principal edgelb-secret
rm -f edgelb-private-key.pem
```

### Install EdgeLB with Secret Name

```
tee edgelb-options.json <<EOF
{
    "service": {
        "secretName": "edgelb-secret",
        "principal": "edgelb-principal"
    }
}
EOF
dcos package install --options=edgelb-options.json edgelb --yes
```

**Note:** The above steps are also available in the script: `scripts/install-enterprise.sh`.

## OSS Install

```
dcos package install edgelb --yes
```

# Usage

## How to read/write the config

See [CONFIG.md](CONFIG.md)

## DC/OS CLI Usage

```
usage: dcos edgelb [<flags>] <command>


Flags:
  -h, --help            Show context-sensitive help.
  -v, --verbose         Enable extra logging of requests/responses
      --force-insecure  Allow unverified TLS certificates when querying service
      --custom-auth-token=DCOS_AUTH_TOKEN
                        Custom auth token to use when querying service
      --custom-dcos-url=DCOS_URI/DCOS_URL
                        Custom cluster URL to use when querying service
      --custom-cert-path=DCOS_CA_PATH/DCOS_CERT_PATH
                        Custom TLS CA certificate file to use when querying service
      --name="edgelb"   Name of the service instance to query

Commands:
  help [<command> ...]
    Show help.


  ping
    Test readiness of edgelb api server


  list [<flags>]
    List the names of all configured pools

    --json  Show unparsed JSON response


  create [<flags>] <pool-file>
    Creates a single pool given a definition file written in JSON or YAML

    --json  Show unparsed JSON response


  show [<flags>] [<pool-name>]
    Shows the pool definition for a given pool name. If pool-name is omitted, all pool configurations are shown

    --reference  Print the configuration reference
    --convert-to-json=CONVERT-TO-JSON
                 Converts local YAML file to JSON
    --json       Show unparsed JSON response


  update [<flags>] <pool-file>
    Updates an existing pool

    --json  Show unparsed JSON response


  delete <pool-name>
    Deletes and uninstalls an existing pool


  status [<flags>] <pool-name>
    List of load-balancer task information associated with the pool such as agent IP address, task ID, etc

    --task-ids  Only print the task ids
    --json      Show JSON summary response


  endpoints [<flags>] <pool-name>
    List of all endpoints for the pool

    --json  Show unparsed JSON response


  lb-config [<flags>] <pool-name>
    Shows the running load-balancer config associated with the pool

    --raw  Show unparsed load-balancer config


  template create <pool-name> <template-file>
    Creates a custom config template for a pool of load-balancers


  template show [<pool-name>]
    Shows the load-balancer config template for an individual pool. If pool-name is omitted, the default template is shown


  template update <pool-name> <template-file>
    Updates a custom config template for a pool of load-balancers


  template delete <pool-name>
    Reverts a custom config template to the default value


  version
    Version information
```

An example pool configuration for `dcos edgelb config <file>` can be found in [examples/config/pool-http.json](examples/config/pool-http.json). Modify the values as necessary.

## Start some marathon apps

```
dcos marathon app add examples/apps/host-httpd.json
dcos marathon app add examples/apps/bridge-nginx.json
```

## Wait for edgelb service

```
until dcos edgelb ping; do sleep 1; done
```

## Create and Install Pool

```
dcos edgelb create examples/config/pool-http.json
```

## List Pool Names

```
dcos edgelb list
```

## Show Pool Configuration

```
dcos edgelb show test-http-pool
```

## Update Pool Configuration

```
dcos edgelb update examples/config/pool-http.json
```

## Show Rendered Load-Balancer Config

```
dcos edgelb lb-config test-http-pool
```

## Get Agent/Task Information About Running Load-Balancers

```
dcos edgelb status test-http-pool
```

## Show Load-Balancer Endpoints

```
dcos edgelb endpoints test-http-pool
```

## Get / Update / Delete Template

```
# get the default template
dcos edgelb template show > default.cfg.ctmpl
# or get the current template for a pool
dcos edgelb template show test-http-pool > custom.cfg.ctmpl
# edit custom.cfg.ctmpl
dcos edgelb template update test-http-pool custom.cfg.ctmpl
# revert back to default
dcos edgelb template delete test-http-pool
```

# Uninstall

Delete each pool with:

```
dcos edgelb pool delete <name>
```

**Note:** This command also uninstalls the loadbalancer pool framework

Uninstall the `apiserver` and `edgelb-pool` cli with:

```
dcos package uninstall edgelb
dcos package uninstall edgelb-pool --cli
```

Remove the universe repos with:

```
dcos package repo remove edgelb-aws
dcos package repo remove edgelb-pool-aws
```

# SSL/TLS Certificates

See [sample-certificates.yaml](examples/config/sample-certificates.yaml)

# Command Line Interface (CLI) Additional Information

There are 2 CLIs. One is for the API Server (`edgelb`), the other is
for the Pool of load balancers (`edgelb-pool`). They both come with help
text (`-h`).

Install the API Server CLI with `dcos package install edgelb --cli`

Install the Pool CLI with `dcos package install edgelb-pool --cli`

The `edgelb` CLI deals with *configuration* and *Pool orchestration*. This is the
general interface to the user.

The `edgelb-pool` CLI gives you more in-depth info on an individual Pool.
It is not often needed, and is more used for debugging. More
information can be found at the [SDK Operations Guide](https://mesosphere.github.io/dcos-commons/operations-guide.html)

## `edgelb-pool` CLI is not working?

You need to set the name of your pool. For example, if your pool is named
`testpool` then you would need to set `--name=dcos-edgelb/pools/testpool`. For example:
```
dcos edgelb-pool --name=dcos-edgelb/pools/testpool pod list
```
