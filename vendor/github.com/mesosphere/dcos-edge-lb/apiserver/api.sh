#!/bin/bash

get() {
    curl localhost:52978/v1/all
}

put() {
    read -r -d '' body <<'EOF'
{
    "pools": [
        {
            "name": "hello"
        }
    ]
}
EOF

    curl -X PUT -H 'Content-Type: application/json' localhost:52978/v1/all -d "$body"
}

badput() {
    read -r -d '' body <<'EOF'
{
    "bad": "fake"
}
EOF

    curl -X PUT -H 'Content-Type: application/json' localhost:52978/v1/all -d "$body"
}

ready() {
    curl -s -o /dev/null -w "%{http_code}" localhost:52978/v1/ping
}

clear() {
    read -r -d '' body <<'EOF'
{
    "pools": [ ]
}
EOF

    curl -X PUT -H 'Content-Type: application/json' localhost:52978/v1/all -d "$body"
}

"$1"
