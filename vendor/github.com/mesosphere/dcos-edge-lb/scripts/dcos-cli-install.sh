#!/bin/sh
CURL='curl --insecure --silent'
DCOS_VERSION=$($CURL $CLUSTER_URL/dcos-metadata/dcos-version.json | jq '.["version"]' | \
    sed 's/["]//' |awk -F'-' '{print $1}' |awk -F'.' '{print $1"."$2}')

$CURL https://downloads.dcos.io/binaries/cli/linux/x86-64/dcos-$DCOS_VERSION/dcos -o dcos
mv dcos /usr/local/bin
chmod +x /usr/local/bin/dcos
dcos cluster setup --username=$CLUSTER_USERNAME --password=$CLUSTER_PASSWORD --no-check $CLUSTER_URL
