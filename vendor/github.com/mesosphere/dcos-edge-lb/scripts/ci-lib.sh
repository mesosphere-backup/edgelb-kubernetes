#!/bin/bash

# $WORKSPACE is provided by the CI system. It should be the only variable
# that we ever expect from the outside.

export GOPATH="$WORKSPACE/go"
export TMPDIR="$WORKSPACE/tmp"
export PATH="${PATH}:${GOPATH}/bin:${TMPDIR}/bin"
export GOPATHACCOUNT="${GOPATH}/src/github.com/mesosphere"
export readonly PROJECT_DIR="${GOPATHACCOUNT}/dcos-edge-lb"
export readonly ARTIFACT_DIR="${PROJECT_DIR}/target"
export readonly TEST_REPORTS="${PROJECT_DIR}/test_results"

status_line() {
    printf "\n### $1 ###\n\n"
}

random_string() {
    num_chars=$1

    # Use a subshell so that the `set` commands do not pollute the parent shell
    (
        set +e +o pipefail

        # Warning: this will error out on linux if set -o pipefail and set -e
        # LC_CTYPE=C is needed on macOS
        random_id=$(cat /dev/urandom | LC_CTYPE=C tr -dc 'a-z0-9' | fold -w "$num_chars" | head -n 1)
        echo "$random_id"
    )
}
