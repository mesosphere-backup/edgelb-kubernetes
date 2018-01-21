# DC/OS Edge Load Balancer Development

This project is comprised of several applications, libraries, and scripts. Each piece can be compiled / deployed individually, but the top level `Makefile` contains tasks which can trigger all downstream builds as well.

# Project Layout

The components included:

* `apiserver/`
  * API server itself
  * `mesos-listener`
  * `github.com/mesosphere/dcos-template`
* `loadbalancer/`
  * HAProxy
* `framework/`
  * `edgelb`: cli and universe packaging for `apiserver`
  * `edgelb-pool`: dcos-commons framework to deploy `loadbalancer` docker image

# Build Options
The majority of this document provides details and rationale for development,
whether that development is on an OSX developer workstation or a Linux workstation
or build box. This solution is tested to ensure both experiences result in a
succesful build. However, the most heavily tested experience is the build as
performed by the build bot machines.

The developer workstation build process can either use the containerized build or
build directly on the developer's workstation using make, after satisfying all of
requirements which are stated here [Requirements](#requirements) as well as
configuring for [uploads](configuring-uploads) and [git](configuring-git).

For details on the containerized build process, which encapsulates the requirements
installation and tooling configuration, and is the same process that is performed
on the build bot machines, see [Containerized Builder](containerized-builder).

<a name="requirements"></a>
# Requirements

Following are dependencies and steps needed to complete a build successfully:

* Go 1.8+
* Python 3.4+
  * [AWS CLI](http://docs.aws.amazon.com/cli/latest/userguide/installing.html) (`pip install awscli`)
  * Wheel (`pip install wheel`)
* Docker 1.12+

<a name="configuring-uploads"></a>
## Configuring Uploads

In order to upload artifacts and images, the following configuration is required:

* Decide on a bucket name for your artifacts, the default is `infinity-artifacts`
* Configure `awscli` the tool and make a bucket for S3 artifacts

```
aws configure # Enter your AWS key and region information
aws s3 mb s3://my-infinity-artifacts
```

* [Login to Docker Registry](https://docs.docker.com/engine/reference/commandline/login/)
* `Makefile` Overrides: Create a top level `local.mk` to override defualt variables. The following are necessary for custom AWS S3 buckets and Docker repositories:

```
DOCKER_REPO = mydockerrepo
S3_BUCKET = my-universe-artifacts
```

<a name="configuring-git"></a>
## Configure git

In order to clone, while supporting two-factor authentication, git protocol is used throughout. If you experience
issues attempting to `git clone`, see [GitHub SSH Help](https://help.github.com/articles/testing-your-ssh-connection/)

Within the continuous integration (CI) environment, the builder uses a SSH key copied into the docker container. The
script used to ensure that this key is authorized for GitHub generally is `scripts/ci-git-ssh.sh`.

# Building

Create all artifacts and images locally:

```
make
```

Upload artifacts and push images:

```
make push
```

## Cleaning

The `make` and `make push` targets are idempotent. The sub tasks in `apiserver/Makefile`, `apiserver/spec/Makefile`, `loadbalancer/Makefile`, and `framework/Makefile` create files such as `.dockerized` or `.pushed` so that these expensive operations are not unintentionally triggered.

To remove all of these files as well as the binaries and artifacts, just run:

```
make clean
```

Each of the individual components also support `make clean` as well for more targetted builds.

## Updating Dependencies

This project's dependencies are managed with `govendor` [https://github.com/kardianos/govendor](https://github.com/kardianos/govendor).

[Handy reference on govendor filters and commands](https://github.com/kardianos/govendor/wiki/Govendor-CheatSheet).

Install govendor:

```
go get -u github.com/kardianos/govendor
```

If a new dependency is added, add existing files from your GOPATH with:

```
govendor add +external
```

To change the referenced version of a dependency, use:

```
govendor fetch <package_name>@<package_revision>
```

Alternatively, modify `vendor/vendor.json` by hand and run:

```
govendor sync
```

See the [govendor quick start](https://github.com/kardianos/govendor#quick-start-also-see-the-faq) for more information.

## Updating dcos-commons

1. Update build.gradle with the new `<dcos-commons-tag>`
1. Update the dcos-commons CLI library

```
cd /path/to/dcos-commons
git checkout master
git pull
git pull --tags
git checkout <dcos-commons-tag>
govendor fetch "github.com/mesosphere/dcos-commons/cli/...@<git_commit_hash_of_tagged_commit>"
```

### Govendor crashes

Govendor will crash if the commit you are referencing is ahead of master, this
may happen with tags where it is one ahead of master, in that case just pick the
master commit.

# Release Process(es)

## New Release
1. Create a new release on github. Follow the convention of the previous
releases.
1. This new tag triggers a Jenkins build
1. Update the documentation (website)
    * swagger.yml: there is a stripped down version of this
    copied there that also needs to be updated.
        * We track the latest release version there in the configuration page.
    * CLI: Update the CLI reference if anything changed.
    * examples: Update the counterpart examples page.

## When updating the CLI
* Update the README

## When updating an example
* Update the CLI tests
    * One tests that the yaml/json are parsed as valid configs.
    * One tests that the yaml/json are parsed the same way (only necessary if there
    is both a yaml and json version). This does not check if they are valid configs.

# Deploying to Cluster

The full manual steps are listed in the Installation section of [README.md](README.md), but there are several `make` tasks to make it easier to change, recompile, and redeploy changes rapidly:

* `make install`: Adds universe repos, installs apiserver using existing `edgelb-options.json`
* `make uninstall`: Deletes test pool, removes universe repos, uninstalls apiserver / pools, cleans docker images, removes framework zk data
* `make reinstall`: Deletes test pool, removes universe repos, uninstalls apiserver / pools, cleans docker images, removes framework zk data, builds artifacts, uploads artifacts, adds universe repos, installs the apiserver
* `make reinstall-[apiserver|framework|framework-edgelb|framework-edgelb-pool]`: Removes specific artifacts / build files and then runs `make reinstall`
* `make test`: Pings apiserver, adds sample marathon apps, stores sample pool config, installs sample pool framework


<a name="containerized-builder"></a>
## Containerized Builder
In order to provide a consistent build experience that even a build bot can
execute, the Dockerfile and (recursive) Makefile and related scripts provide
the build steps from a bare OS provisioned machine (container) to a machine
that is able to execute any of the make targets within this project as well
as a locally executable set of Make targets to build the container image
(Make target: `container-build`) or run a build within a container (Make
target: `container-run`).

The container is run with the following volumes:

| Host                 | Container            | Purpose |
|----------------------|----------------------|---------|
| /var/run/docker.sock | /var/run/docker.sock | Share the Docker socket with the host, to reduce out the complexity of docker in docker |
| SSH_DIR (~/.ssh)     | /root/.ssh           | Share the SSH keys to enable GitHub authentication |
| AWS_DIR (~/.aws)     | /root/.aws           | Share the AWS credentials and configuration to enable asset publication |

The environment variables used by both container-build and container-run are
covered by the following matrix (since many are used in both):

| Override | Build | Run | Variable | Purpose |
|----------|-------|-----|----------|---------|
| [ ]      | [X]   | [X] | `PROJECTDIR` | Path to the project, in the `GOPATH`, as is idiomatic for golang |
| [ ]      | [X]   | [X] | `GOPATH`     | Path to the root of all golang projects |
| [ ]      | [X]   | [ ] | `GOROOT`     | Path to the root of all golang tooling  |
| [ ]      | [X]   | [X] | `HOMEDIR`    | Path to the user's home, used especially for tools that expect configs stored relative to HOMEDIR |
| [ ]      | [X]   | [X] | `SSHDIR`     | Path to the directory that will contain the user's GitHub SSH key |
| [X]      | [X]   | [ ] | `SSH_GIT_KEY` | Filename of the user's GitHub SSH Key |
| [ ]      | [X]   | [X] | `AWSDIR`     | Path to the directory that will contain the user's AWS credentials and configuration, ie for region specification ]
| [X]      | [X]   | [X] | `DOCKER_USERNAME` | Docker registry credentials, username |
| [X]      | [X]   | [X] | `DOCKER_PASSWORD` | Docker registry credentials, password |
| [X]      | [X]   | [X] | `DOCKER_REPO`     | Docker registry repository, where the built container images will be published |
| [ ]      | [ ]   | [X] | `CONTAINER_RUN_COMMAND` | Command to run on the container, defaults to the CI build script `ci-build` |
| [X]      | [ ]   | [X] | `CONTAINER_MAKE_TARGET` | Make target to run, assuming a CI build, defaults to `push` |
| [X]      | [ ]   | [X] | `CLUSTER_URL`           | URL of the DC/OS cluster used for build testing |
| [X]      | [ ]   | [X] | `CLUSTER_USERNAME`      | DC/OS cluster credentials, username |
| [X]      | [ ]   | [X] | `CLUSTER_PASSWORD`      | DC/OS cluster credentials, password |
