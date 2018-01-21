#!/usr/bin/env bash

PUB=""

: ${DCOS_USER:="core"}

for i in $(dcos node --json| jq -r '.[] | select(.attributes.public_ip == "true") | .hostname'); do
    OUT=$(dcos node ssh "--user=$DCOS_USER" --option StrictHostKeyChecking=no --master-proxy --private-ip=$i '/opt/mesosphere/bin/detect_ip_public' 2> /dev/null)
    PUB="$PUB$(echo $OUT | cut -d'C' -f1)\n"
done

printf "$PUB"
