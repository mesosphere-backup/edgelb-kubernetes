#!/bin/bash

set -euo pipefail

ORIGDIR="$(pwd)"

source "$(dirname $0)/ci-lib.sh"

# Go to project dir early in order to do
status_line "cd $PROJECT_DIR"
cd "$PROJECT_DIR"
CUR_GIT_TAG="$(git describe --tags)"
cd -

RELEASE_SCRIPT="$(dirname $0)/ci-release.sh"
BUCKET="edge-lb-infinity-artifacts"
VERSION="$CUR_GIT_TAG"
S3_DIR_PATH="permanent/tag/$VERSION"

status_line "Tag BUCKET=$BUCKET VERSION=$VERSION S3_DIR_PATH=$S3_DIR_PATH"
"$RELEASE_SCRIPT" "$BUCKET" "$VERSION" "$S3_DIR_PATH"

status_line "Returning to $ORIGDIR"
cd "$ORIGDIR"
