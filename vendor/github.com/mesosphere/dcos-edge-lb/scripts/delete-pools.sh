#!/usr/bin/env bash

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

edgelb_already_uninstalled () {
    echo 'edgelb is already uninstalled, so pools must be removed'
    exit 0
}

dcos edgelb version >/dev/null 2>&1 || edgelb_already_uninstalled

for i in $(dcos edgelb list --json | jq -r '.[].name'); do
    dcos edgelb delete $i
done

# poll for pool tasks since the pool's complete isn't otherwise observable afaict
while dcos task |grep 'edgelb.*[-]pool[.]' >/dev/null 2>&1; do sleep 0.1; done
