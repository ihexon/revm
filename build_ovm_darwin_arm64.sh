#! /usr/bin/env bash
set -e
set -o pipefail

readonly RED="\033[31m"
readonly YELLOW="\033[33m"
readonly GREEN="\033[32m"
readonly RESET="\033[0m"

log_err() {
    echo -e "${RED}[ERROR]${RESET} $*" >&2
    exit 100
}

log_warn() {
    echo -e "${YELLOW}[WARN]${RESET} $*" >&2
}

log_info() {
    echo -e "${GREEN}[INFO]${RESET} $*"
}

init_workspace() {
    WORKSPACE="$(realpath "$(dirname "$0")")"
    OUTDIR="$WORKSPACE/out"

    export WORKSPACE
    export OUTDIR
    rm -rf "$OUTDIR"

    log_info "Workspace: $WORKSPACE"
}

build_darwin_arm64() {
    cd "$WORKSPACE/guest-agent"
    bash -x build.sh "linux" "arm64" "$WORKSPACE/pkg/static_resources/guest-agent/guest-agent"

    mkdir -p "$OUTDIR/.deps/e2fsprogs" && cd "$OUTDIR/.deps/e2fsprogs"
    wget -q https://github.com/ihexon/revm-assets/releases/download/v2.0.0/e2fsprogs-Darwin-arm64.tar.zst --output-document=- | tar -xvz
    cp -av sbin/blkid "$WORKSPACE/pkg/static_resources/e2fsprogs/blkid"
    cp -av sbin/tune2fs "$WORKSPACE/pkg/static_resources/e2fsprogs/tune2fs"
    cp -av sbin/mke2fs "$WORKSPACE/pkg/static_resources/e2fsprogs/mke2fs"
    cp -av sbin/e2fsck "$WORKSPACE/pkg/static_resources/e2fsprogs/e2fsck"

    mkdir -p "$OUTDIR/.deps/libkrun" && cd "$OUTDIR/.deps/libkrun"
    wget -q https://github.com/ihexon/revm-assets/releases/download/v2.0.0/libkrun-Darwin-arm64.tar.zst --output-document=- | tar -xvz

    mkdir -p "$OUTDIR/.deps/libkrunfw" && cd "$OUTDIR/.deps/libkrunfw"
    wget -q https://github.com/ihexon/revm-assets/releases/download/v2.0.0/libkrunfw-Darwin-arm64.tar.zst --output-document=- | tar -xvz

    cd "$WORKSPACE"
    commit_id="$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")"
    git_tag="$(git describe --tags --abbrev=0 2>/dev/null || echo "unknown")"

    log_info "Version: ${commit_id}, Commit: ${commit_id}"

    wget https://github.com/ihexon/revm-assets/releases/download/v2.0.0/alpine-rootfs-Linux-aarch64.tar.zst \
        --output-document="$WORKSPACE/pkg/static_resources/rootfs/rootfs.tar.zst"

    local ldflags="-X linuxvm/pkg/define.Version=$git_tag -X linuxvm/pkg/define.CommitID=$commit_id"

    local target_bin="$OUTDIR/bin/revm"
    go build -ldflags="$ldflags" -v -o "$target_bin" ./cmd

    cp -av "$OUTDIR/.deps/libkrun/lib/"* "$(dirname "$target_bin")"
    cp -av "$OUTDIR/.deps/libkrunfw/lib/"* "$(dirname "$target_bin")"

    codesign --entitlements revm.entitlements --force -s - "$target_bin"

    # restore the empty file
    cp /dev/null "$WORKSPACE/pkg/static_resources/guest-agent/guest-agent"
    cp /dev/null "$WORKSPACE/pkg/static_resources/rootfs/rootfs.tar.zst"
    cp /dev/null "$WORKSPACE/pkg/static_resources/e2fsprogs/blkid"
    cp /dev/null "$WORKSPACE/pkg/static_resources/e2fsprogs/tune2fs"
    cp /dev/null "$WORKSPACE/pkg/static_resources/e2fsprogs/e2fsck"
    cp /dev/null "$WORKSPACE/pkg/static_resources/e2fsprogs/mke2fs"
}

main() {
    init_workspace
    build_darwin_arm64
}

main "$@"
