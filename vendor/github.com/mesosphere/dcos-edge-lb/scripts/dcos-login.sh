#!/usr/bin/env bash

if [ $# -lt 1 ]; then
    printf "Syntax: $0 <dashboard_url>\n"
    exit 1
fi

DASHBOARD_URL=$1

printf DASHBOARD_URL=$DASHBOARD_URL

printf "Configuring local dcos...\n"

dcos cluster setup $DASHBOARD_URL
