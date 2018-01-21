#!/bin/bash

set -euo pipefail

ORIGDIR="$(pwd)"

SCRIPT_DIR="$(dirname $0)"

source "$(dirname $0)/ci-lib.sh"

status_line "cd $PROJECT_DIR"
cd "$PROJECT_DIR"

status_line "Cleaning up"
make delete-cluster-integration

status_line "Returning to $ORIGDIR"
cd "$ORIGDIR"
