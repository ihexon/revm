#!/usr/bin/env bash

set -euo pipefail
set -x

cd "$(dirname "$0")"

REVM=./out/bin/revm

run_tests() {
    local net=$1
    local workspace="/tmp/my_workspace_$RANDOM"

    echo "=== Testing network mode: $net ==="

    "$REVM" run --network "$net" --workspace "$workspace" --log-level debug -- ifconfig

    echo ifconfig | "$REVM" run --network "$net" --workspace "$workspace" --log-level debug -- sh

    echo ifconfig | sh -c "\"$REVM\" run --network \"$net\" --workspace \"$workspace\" --log-level debug -- sh" | grep --color "127.0.0.1"

    # sleep two second wait for guest network ready
    echo "sleep 2 && nslookup bing.com" | sh -c "\"$REVM\" run --network \"$net\" --workspace \"$workspace\" --log-level debug -- sh"

    rm -rf "$workspace"
}

for mode in "tsi" "gvisor"; do
    run_tests "$mode"
done
