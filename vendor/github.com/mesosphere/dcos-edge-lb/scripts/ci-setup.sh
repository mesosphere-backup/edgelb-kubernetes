#!/bin/bash

set -euo pipefail

ORIGDIR="$(pwd)"

source "$(dirname $0)/ci-lib.sh"

status_line "Setting up CI workspace"

service docker start

# This stuff is necessary because jenkins doesn't actually clean anything up.
rm -rf "$GOPATH"
rm -rf "$TMPDIR"
mkdir -p "$TMPDIR"
mkdir -p "$GOPATHACCOUNT"
mv dcos-edge-lb "$GOPATHACCOUNT/"

# There's a strange bug in Jenkins where this file does not exist sometimes?
# Maybe since we do a `mv` it's just slow I/O or something?
CI_PR_SH="$PROJECT_DIR/scripts/ci-pr.sh"
until [ -n "$(cat $CI_PR_SH || true)" ]; do
    echo "could not find $CI_PR_SH"
    sleep 1
done
