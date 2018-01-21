#!/bin/bash
set -e

# capture anonymous metrics for reporting
curl https://mesosphere.com/wp-content/themes/mesosphere/library/images/assets/sdk/build-sh-start.png >/dev/null 2>&1

# Custom
export S3_URL=${S3_URL}
export ARTIFACT_DIR=${ARTIFACT_DIR}
export TEMPLATE_LBTASK_DOCKER=${LBTASK_DOCKER}
export TEMPLATE_LBTASK_VERSION=${LBTASK_VERSION}
# Default
FRAMEWORK_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
BUILD_DIR=$FRAMEWORK_DIR/build/distributions
PUBLISH_STEP=${1-none}
export REPO_NAME="$(basename $FRAMEWORK_DIR)"
export BUILD_BOOTSTRAP=no
# export TOOLS_DIR=${FRAMEWORK_DIR}/tools
export CLI_DIR=${FRAMEWORK_DIR}/cli
# export ORG_PATH=github.com/$REPO_NAME
export ORG_PATH=github.com/mesosphere/dcos-edge-lb/framework

# Custom
export TOOLS_DIR="$( cd "$FRAMEWORK_DIR/../tools" && pwd )"
export REPO_ROOT_DIR=$FRAMEWORK_DIR

${TOOLS_DIR}/build_framework.sh $PUBLISH_STEP $REPO_NAME $FRAMEWORK_DIR $BUILD_DIR/$REPO_NAME-scheduler.zip

# capture anonymous metrics for reporting
curl https://mesosphere.com/wp-content/themes/mesosphere/library/images/assets/sdk/build-sh-finish.png >/dev/null 2>&1
