#!/bin/bash

SCRIPT_DIR="$(dirname $0)"

source "$SCRIPT_DIR/ci-lib.sh"

set -a

### YOU MAY BE WONDERING WHAT'S WITH THESE STRANGE DIRECTORY/PATH NAMES ###
# They are all shortened to the max because linux has a 127 character limit
# for the "shebang" line (e.g. #!/bin/bash) which virtualenv uses. So the
# path to the virtualenv is as short as possible.

EXAMPLES_DIR="$SCRIPT_DIR/../examples"
INTEGRATION_TEST_DIR="$SCRIPT_DIR/../itests"
REQUIREMENTS_TXT="$INTEGRATION_TEST_DIR/requirements.txt"
PYTEST_DIR="$INTEGRATION_TEST_DIR/tests"
WORKDIR="$INTEGRATION_TEST_DIR/_b"

WORKBIN="$WORKDIR/bin"
PYTEST_WORKDIR="$WORKDIR/pytest_tmp"
DCOS_LAUNCH_VENV="$WORKDIR/v"

DCOS_LAUNCH_DIR="$WORKDIR/dcos_launch_dir"
DCOS_LAUNCH_CONFIG="$DCOS_LAUNCH_DIR/config.yaml"

## 1.11 EE Permissive
# DCOS_CLI_VERSION="dcos-1.11"
# echo CFM_TEMPLATE_URL=${CFM_TEMPLATE_URL:="https://s3.amazonaws.com/downloads.mesosphere.io/dcos-enterprise/testing/master/cloudformation/ee.single-master.cloudformation.json"}

## 1.10 EE Permissive
DCOS_CLI_VERSION="dcos-1.10"
echo CFM_TEMPLATE_URL=${CFM_TEMPLATE_URL:="https://s3.amazonaws.com/downloads.mesosphere.io/dcos-enterprise/stable/cloudformation/ee.single-master.cloudformation.json"}

## 1.11 EE Strict
# DCOS_CLI_VERSION="dcos-1.11"
# echo CFM_TEMPLATE_URL=${CFM_TEMPLATE_URL:="https://s3.amazonaws.com/downloads.mesosphere.io/dcos-enterprise-aws-advanced/testing/master/strict/cloudformation/ee.single-master.cloudformation.json"}

## 1.10 EE Strict
# DCOS_CLI_VERSION="dcos-1.10"
# echo CFM_TEMPLATE_URL=${CFM_TEMPLATE_URL:="???"}


echo CLUSTER_URL=${CLUSTER_URL:="uninitialized"}
echo INTEGRATION_ENV_LOADED=${INTEGRATION_ENV_LOADED:="false"}
echo DCOS_USERNAME=${DCOS_USERNAME:="bootstrapuser"}
echo DCOS_PASSWORD=${DCOS_PASSWORD:="deleteme"}

APILIST="\
bootstrap \
debug_run_integration \
run_integration \
run_single_test \
run_lint \
delete_cluster \
"

load_env() {
    if [ "$INTEGRATION_ENV_LOADED" == "true" ]; then
        echo "Integration env already loaded"
        return
    fi

    if [ -z ${VIRTUAL_ENV+x} ]; then
        # Not inside virtualenv, this is ok
        true
    else
        echo "Already running inside virtualenv $VIRTUAL_ENV. Cannot nest virtualenvs."
        return 1
    fi

    set +u
    . "$DCOS_LAUNCH_VENV/bin/activate"
    set -u

    export PATH=$WORKBIN:$PATH
    echo "PATH=$PATH"
    export DCOS_DIR="$WORKDIR/dcos_cli_dir"
    echo "DCOS_DIR=$DCOS_DIR"
    export PYTEST_WORKDIR
    echo "PYTEST_WORKDIR=$PYTEST_WORKDIR"
    export EXAMPLES_DIR
    echo "EXAMPLES_DIR=$EXAMPLES_DIR"

    INTEGRATION_ENV_LOADED="true"
}

wrapped_dcos_launch() {
    # Use a subshell so `cd` doesn't pollute the environment
    (
        cd "$DCOS_LAUNCH_DIR"
        dcos-launch $@
    )
}

download_cli() {
    if [ "$(uname)" = "Linux" ]; then
        local _cli_os="linux"
    elif [ "$(uname)" = "Darwin" ]; then
        local _cli_os="darwin"
    fi

    curl -L -o "$WORKBIN/dcos" "https://downloads.dcos.io/binaries/cli/${_cli_os}/x86-64/${DCOS_CLI_VERSION}/dcos"
    chmod 755 "$WORKBIN/dcos"
}

bootstrap() {
    type curl
    type jq

    rm -rf "$WORKDIR"
    mkdir -p "$WORKDIR"
    mkdir -p "$WORKBIN"
    mkdir -p "$PYTEST_WORKDIR"
    mkdir -p "$DCOS_LAUNCH_DIR"

    download_cli

    python3 -m venv "$DCOS_LAUNCH_VENV"
    load_env
    # Install wheel separately as the executable is needed to install other
    # pip packages.
    pip3 install wheel
    pip3 install -r "$REQUIREMENTS_TXT"

    type "$WORKBIN/dcos"
    type dcos
}

maybe_launch_cluster() {
    status_line "Maybe launch cluster"

    if [ "$CLUSTER_URL" != "uninitialized" ]; then
        echo "Using provided CLUSTER_URL as cluster: $CLUSTER_URL"
        return
    fi
    echo "CLUSTER_URL is empty/unset, launching new cluster."

    random_id="$(random_string 10)"

    cat <<EOF > "$DCOS_LAUNCH_CONFIG"
---
launch_config_version: 1
deployment_name: dcos-ci-test-edge-lb-$random_id
template_url: $CFM_TEMPLATE_URL
provider: aws
aws_region: us-west-2
template_parameters:
    KeyName: default
    AdminLocation: 0.0.0.0/0
    PublicSlaveInstanceCount: 1
    SlaveInstanceCount: 1
    DefaultInstanceType: m4.large
ssh_user: core
EOF
    time wrapped_dcos_launch create
    time wrapped_dcos_launch wait
    wrapped_dcos_launch describe
    # jq emits json strings by default: "value".  Use --raw-output to get value without quotes
    CLUSTER_URL=https://$(wrapped_dcos_launch describe | jq -r .masters[0].public_ip)
    echo "CLUSTER_URL=$CLUSTER_URL"
}

delete_cluster() {
    load_env
    time wrapped_dcos_launch delete
}

cluster_ids() {
    if ! dcos cluster list >/dev/null 2>&1; then
        return
    fi

    dcos cluster list | tail -n +2 | tr -s ' ' | sed -e "s/^ *//" |  cut -d' ' -f2
}

configure_cluster() {
    status_line "Configuring cluster"

    echo "Removing old clusters"
    for id in $(cluster_ids); do
        echo "remove $id"
        dcos cluster remove $id
    done

    echo "Authenticating"
    dcos cluster setup --no-check --username=$DCOS_USERNAME --password=$DCOS_PASSWORD $CLUSTER_URL
}

debug_run_integration() {
    run_integration -s
}

run_integration() {
    status_line "Running integration tests"
    load_env

    maybe_launch_cluster
    configure_cluster

    run_lint

    wrapped_pytest -vv "$PYTEST_DIR/" $@
}

wrapped_pytest() {
    # Locale needed in some environments (jenkins). The values of locales are
    # also not consistent between linux and macOS.

    if [ "$(uname)" = "Linux" ]; then
        LC_ALL=C.UTF-8 \
            LANG=C.UTF-8 \
            py.test $@
    elif [ "$(uname)" = "Darwin" ]; then
        py.test $@
    fi
}

run_lint() {
    status_line "Running integration test linter"
    load_env

    flake8 --verbose "$PYTEST_DIR/"
    isort --recursive --check-only --diff --verbose "$PYTEST_DIR/"
}

run_single_test() {
    load_env

    configure_cluster

    run_lint
    wrapped_pytest -s -vv "$PYTEST_DIR/$@"
}

### main

comm=$1
shift
foundMatch=false
if [ -n "$comm" ]; then
    for func in $APILIST; do
        if [ "$comm" = "$func" ]; then
            foundMatch=true
        fi
    done
fi
if [ "$foundMatch" = true ]; then
    set -euo pipefail
    "${comm}" "$@"
else
    echo "Command not found, please try one of:"
    for comm in $APILIST; do
        echo $comm
    done
fi
