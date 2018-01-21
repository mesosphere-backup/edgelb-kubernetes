#!/usr/bin/env bash

pingAPI() {
    until dcos edgelb ping; do sleep 1; done
}

testPool() {
    for i in $(dcos node --json| jq -r '.[] | select(.attributes.public_ip == "true") | .hostname'); do
        OUT=$(dcos node ssh --option StrictHostKeyChecking=no --master-proxy --private-ip=$i '/opt/mesosphere/bin/detect_ip_public' 2> /dev/null)
        PUB="$(echo $OUT | cut -d'C' -f1)"
        until curl -f http://$PUB; do sleep 1; done
    done
}

pingAPI
testPool
