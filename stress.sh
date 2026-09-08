#!/usr/bin/env bash

URL="http://127.0.0.1:3000/"
REQUESTS=20
IPS=1
START=$(date +%s%N)
timestamp() {
    local now
    now=$(date +%s%N)
    echo "$(((now - START)/1000000))ms"
}
for i in $(seq 2 $((IPS + 1))); do
    IP="127.0.0.$i"

    for j in $(seq 1 "$REQUESTS"); do
        (
            CODE=$(curl \
                --silent \
                --output /dev/null \
                --write-out "%{http_code}" \
                --interface "$IP" \
                "$URL")

            printf '%s %s -> %s\n' \
                "$(timestamp)" \
                "$IP" \
                "$CODE"
        ) &
    done
done

wait