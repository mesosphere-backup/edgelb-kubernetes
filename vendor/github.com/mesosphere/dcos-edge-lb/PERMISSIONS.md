# Components

* Edge-LB API Server: `/dcos-edgelb/api` (`api.edgelb.marathon.l4lb.thisdcos.directory:80`)
* Edge-LB Pool Schedulers: `/dcos-edgelb/pools/<poolname>` (`api.dcos-edgelbpools<poolname>.marathon.l4lb.thisdcos.directory:80`)
* Edge-LB Pool Servers: Tasks under the scheduler named `edgelb-pool-<instance-num>-server`

# Communication

The Edge-LB API Server connects to the following endpoints inside the DC/OS cluster:

On Startup:

* Mesos Master Operator API POST: `https://leader.mesos:5050/api/v1`
    * OSS DC/OS: Connects using cluster's CA cert only
    * Enterprise DC/OS Permissive: Authenticates with Edge-LB API Server service account (optional)
    * Enterprise DC/OS Strict: Authenticates with Edge-LB API Server service account (required)

On Edge-LB Pool Update and backend app / task changes (without user interaction):

* DC/OS Admin Router (Edge-LB Pool Scheduler Reload HAProxy Server Config) POST: `https://leader.mesos/service/<poolnamespace>/<poolname>/v1/plans/deploy/restart?phase=sidecar-deploy`
    * OSS DC/OS: Not supported
    * Enterprise DC/OS Permissive: Authenticates with Edge-LB API Server service account (required)
    * Enterprise DC/OS Strict: Authenticates with Edge-LB API Server service account (required)

On Edge-LB Pool create / update / delete (initiated by CLI user)

* DC/OS Admin Router (Cosmos Install Pool Frameworks) POST: `https://leader.mesos/package/install`
* DC/OS Admin Router (Cosmos Uninstall Pool Frameworks) POST: `https://leader.mesos/package/uninstall`
* DC/OS Admin Router (Cosmos Describe Pool Frameworks) POST: `https://leader.mesos/package/render`
* DC/OS Admin Router (Marathon App) GET: `https://leader.mesos/marathon/v2/apps/dcos-edgelb/pools/<poolname>`
* DC/OS Admin Router (Marathon App) PUT: `https://leader.mesos/marathon/v2/apps/dcos-edgelb/pools/<poolname>`
    * OSS DC/OS: Authenticates with CLI user's `core.dcos_acs_token`
    * Enterprise DC/OS Permissive: ^
    * Enterprise DC/OS Strict: ^
    
**Note**: If Edge-LB Pool create / update / delete are called without a token (as would be the case if the Edge-LB API server's endpoints were called from outside the CLI), they will still succeed if a service account is configured. If one is not configured, these calls would fail. This situation would always result in failure for OSS DC/OS because service accounts do not exist there.

# Permissions

Edge-LB is installed as a DC/OS service, not as a built-in component. As such, it has some additional permissions that are required for use.

All actions may also be performed by users with the `dcos:superuser` permission.

# Install Permissions

In order to install Edge-LB, the user must have the following permissions:

- `dcos:adminrouter:package`
- `dcos:adminrouter:service:edgelb`
- `dcos:adminrouter:service:marathon`
- `dcos:service:marathon:marathon:services:/dcos-edgelb`

# Multitenant Usage / User Permissions

To grant limited permission to manage only a single Edge-LB pool, the user must have the following permissions:

- `dcos:adminrouter:service:marathon`
- `dcos:adminrouter:package`
- `dcos:service:marathon:marathon:services:/dcos-edgelb/pools/<pool-name>`
- `dcos:adminrouter:service:edgelb:/pools/<poolname>`

Specific actions can also be locked by permissions:

- List Pools: 
    - (GET) `dcos:adminrouter:service:edgelb:/config`
- Read Pool:
    - (GET) `dcos:adminrouter:service:edgelb:/pools/<poolname>`
- Create V1 Pool:
    - (POST) `dcos:adminrouter:service:edgelb:/v1/loadbalancers`
    - (POST) `dcos:adminrouter:package`
- Update V1 Pool:
    - (PUT) `dcos:adminrouter:service:edgelb:/v1/loadbalancers/<poolname>`
    - (POST) `dcos:adminrouter:package`
    - (PUT) `dcos:service:marathon:marathon:services:/dcos-edgelb/pools/<pool-name>`
- Create V2 Pool:
    - (POST) `dcos:adminrouter:service:edgelb:/v2/pools`
    - (POST) `dcos:adminrouter:package`
- Update V2 Pool:
    - (PUT) `dcos:adminrouter:service:edgelb:/v2/pools/<poolname>`
    - (POST) `dcos:adminrouter:package`
    - (PUT) `dcos:service:marathon:marathon:services:/dcos-edgelb/pools/<pool-name>`
- Delete Pool
    - (DELETE): `dcos:adminrouter:service:edgelb:/v2/pools/<poolname>`
    - (POST) `dcos:adminrouter:package`

# Service Account Permissions

In Enterprise DC/OS with strict mode security, Edge-LB must be configured to use a service account with the following permissions:

- Subscribe to operator API: `dcos:mesos:master:???`
- Register as framework: `dcos:mesos:master:framework:role`

Pools can also be configured with their own service account (per pool) separate from the API server's. These serice accounts are only necessary for strict mode security, and will only require this permission:

- Register as framework: `dcos:mesos:master:framework:role`

If a pool's configuration does not specify a service account, the API server's is used.
