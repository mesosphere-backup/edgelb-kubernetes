#!/usr/bin/env bash

set -o pipefail errexit nounset

dcos package install dcos-enterprise-cli --yes --cli
dcos security org service-accounts keypair edgelb-private-key.pem edgelb-public-key.pem
dcos security org service-accounts create -p edgelb-public-key.pem -d "edgelb service account" edgelb-principal || true
dcos security org service-accounts show edgelb-principal
dcos security org groups add_user superusers edgelb-principal
dcos security secrets create-sa-secret --strict edgelb-private-key.pem edgelb-principal edgelb-secret || true
dcos security secrets list /
dcos security secrets get /edgelb-secret --json | jq -r .value | jq
rm -f edgelb-private-key.pem

tee edgelb-options.json <<EOF
{
    "service": {
        "secretName": "edgelb-secret",
        "principal": "edgelb-principal",
        "forcePull": true,
        "cpus": 0.3,
        "mem": 300
    }
}
EOF

echo "Waiting for deployments to finish"
until ! dcos marathon deployment list > /dev/null 2>&1; do
    sleep 1
done

dcos package install --options=edgelb-options.json edgelb --yes
