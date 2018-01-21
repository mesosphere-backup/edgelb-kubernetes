#!/usr/bin/env bash

set -o pipefail errexit nounset

tee edgelb-options.json <<EOF
{
    "service": {
        "secretName": "",
        "forcePull": true
    }
}
EOF
dcos package install --options=edgelb-options.json edgelb --yes
dcos edgelb >/dev/null 2>&1 ||
    dcos package install edgelb --cli --yes
