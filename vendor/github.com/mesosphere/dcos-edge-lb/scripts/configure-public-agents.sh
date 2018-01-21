#!/usr/bin/env bash

ssh_run() {
    dcos node ssh "--user=$DCOS_USER" --option "LogLevel=QUIET" --option "UserKnownHostsFile=/dev/null" --option "StrictHostKeyChecking=no" --master-proxy "$@"
}

printf "Configuring public nodes...\n"

PUB="Public Agent IPs: \n"

: ${DCOS_USER:="core"}

for i in $(dcos node --json| jq -r '.[] | select(.attributes.public_ip == "true") | .hostname'); do
    printf "Configuring public node $i\n"
    ssh_run --private-ip=$i 'sudo systemctl stop dcos-mesos-slave-public.service'
    ssh_run --private-ip=$i 'sudo sed -i "s,MESOS_DEFAULT_ROLE=slave_public,MESOS_DEFAULT_ROLE=*,g" /opt/mesosphere/etc/mesos-slave-public'
    ssh_run --private-ip=$i 'sudo rm -f /var/lib/mesos/slave/meta/slaves/latest'
    ssh_run --private-ip=$i 'sudo systemctl start dcos-mesos-slave-public.service'
    OUT=$(ssh_run --private-ip=$i '/opt/mesosphere/bin/detect_ip_public')
    PUB="$PUB$(echo $OUT | cut -d'C' -f1)\n"
done

printf "\n$PUB\n"
