#!/bin/bash

set -euo pipefail

ORIGDIR="$(pwd)"

SCRIPT_DIR="$(dirname $0)"

source "$SCRIPT_DIR/ci-lib.sh"

run_push() {
    # Pushing to docker is pretty glitchy so try up to 3 times
    make push || sleep 5 && make push || sleep 5 && make push
}

BUCKET="$1"
VERSION="$2"
S3_DIR_PATH="$3"

status_line "cd $PROJECT_DIR"
cd "$PROJECT_DIR"

status_line "Setting up environment"
## Creating the bucket is not required every PR
# aws s3 mb "s3://$BUCKET"

cat << EOF > "local.mk"
S3_BUCKET = $BUCKET
DOCKER_VERSION = $VERSION
S3_DIR_PATH = $S3_DIR_PATH
EOF

docker login -u "${DOCKER_HUB_USERNAME}" -p "${DOCKER_HUB_PASSWORD}" -e "${DOCKER_HUB_EMAIL}"

make bootstrap-integration
# Lint early on to make sure python is set up properly
make lint-integration

status_line "Running PR building"
"$SHELL" "$(dirname $0)/ci-localtests.sh"

status_line "Pushing"
time run_push

make run-integration

status_line "Returning to $ORIGDIR"
cd "$ORIGDIR"
