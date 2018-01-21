#!/bin/bash
set -e

# capture anonymous metrics for reporting
curl https://mesosphere.com/wp-content/themes/mesosphere/library/images/assets/sdk/build-sh-start.png >/dev/null 2>&1

# Default
FRAMEWORK_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
BUILD_DIR=$FRAMEWORK_DIR/build/distributions
PUBLISH_STEP=${1-none}
export REPO_NAME="$(basename $FRAMEWORK_DIR)"
export BUILD_BOOTSTRAP=no
export TOOLS_DIR="$( cd "$FRAMEWORK_DIR/../tools" && pwd )"
export CLI_DIR=${FRAMEWORK_DIR}/cli
# export ORG_PATH=github.com/$REPO_NAME
export ORG_PATH=github.com/mesosphere/dcos-edge-lb/framework
# ${FRAMEWORK_DIR}/tools/build_framework.sh $PUBLISH_STEP $REPO_NAME $FRAMEWORK_DIR

echo TOOLS_DIR=$TOOLS_DIR

# Custom (from build_framework.sh)
export S3_URL=${S3_URL}
export ARTIFACT_DIR=${ARTIFACT_DIR}
export TEMPLATE_APISERVER_DOCKER=${APISERVER_DOCKER}
export TEMPLATE_APISERVER_VERSION=${APISERVER_VERSION}
INCLUDE_BOOTSTRAP=""
FRAMEWORK_NAME=$REPO_NAME
ARTIFACT_FILES=""
export REPO_ROOT_DIR=$FRAMEWORK_DIR

UNIVERSE_DIR=${UNIVERSE_DIR:=${FRAMEWORK_DIR}/universe}
CLI_EXE_NAME=${CLI_EXE_NAME:=dcos-${FRAMEWORK_NAME}}

source $TOOLS_DIR/init_paths.sh
# CLI (Go):
REPO_CLI_RELATIVE_PATH="$(echo $CLI_DIR | cut -c $((2 + ${#REPO_ROOT_DIR}))-)"
${TOOLS_DIR}/build_cli.sh ${CLI_EXE_NAME} ${CLI_DIR} ${REPO_CLI_RELATIVE_PATH}
if [ $? -ne 0 ]; then
    _notify_github failure "CLI build failed"
    exit 1
fi

case "$PUBLISH_STEP" in
    aws)
        echo "Uploading to S3"
        PUBLISH_SCRIPT=${TOOLS_DIR}/publish_aws.py
        ;;
    *)
        echo "---"
        echo "Build complete, skipping publish step."
        echo "Use one of the following additional arguments to get something that runs on a cluster:"
        echo "- 'aws': Upload the build to S3."
        ;;
esac

if [ -n "$PUBLISH_SCRIPT" ]; then
    $PUBLISH_SCRIPT \
        ${FRAMEWORK_NAME} \
        ${UNIVERSE_DIR} \
        ${INCLUDE_BOOTSTRAP} \
        ${CLI_DIR}/dcos-*/dcos-* \
        ${CLI_DIR}/dcos-*/*.whl \
        ${ARTIFACT_FILES}
fi

# Default
# capture anonymous metrics for reporting
curl https://mesosphere.com/wp-content/themes/mesosphere/library/images/assets/sdk/build-sh-finish.png >/dev/null 2>&1
