#! /usr/bin/env bash
set -ex

set_env_linux_arm64() {
    export GOOS="linux"
    export GOARCH="arm64"
}

set_env_linux_amd64() {
    export GOOS="linux"
    export GOARCH="amd64"
}

main() {
    local target_bin="$3"
    local plt="$1"
    local arch="$2"
    if [[ "$plt" == "linux" ]] && [[ "$arch" == "arm64" ]] || [[ "$arch" == "aarch64" ]]; then
        set_env_linux_arm64
    fi

    if [[ "$plt" == "linux" ]] && [[ "$arch" == "x86_64" ]] || [[ "$arch" == "amd64" ]]; then
        set_env_linux_amd64
    fi

    if [[ -z "$target_bin" ]]; then
        target_bin="/dev/null"
    fi

    go build \
        -ldflags="-s -w" \
        -o "$target_bin" main.go
}

main "$@"
