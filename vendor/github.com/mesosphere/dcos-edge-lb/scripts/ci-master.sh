#!/bin/bash

set -euo pipefail

ORIGDIR="$(pwd)"

source "$(dirname $0)/ci-lib.sh"

RELEASE_SCRIPT="$(dirname $0)/ci-release.sh"
BUCKET="edge-lb-infinity-artifacts"
VERSION="master"
S3_DIR_PATH="autodelete7d/$VERSION"

status_line "Master BUCKET=$BUCKET VERSION=$VERSION S3_DIR_PATH=$S3_DIR_PATH"
"$RELEASE_SCRIPT" "$BUCKET" "$VERSION" "$S3_DIR_PATH"

status_line "Returning to $ORIGDIR"
cd "$ORIGDIR"
