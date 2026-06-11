#!/usr/bin/env bash
set -xe
set -o pipefail

# shellcheck source=common.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

PKG_NAME="alpine-rootfs"
WORKSPACE="$DEPS_WORK_DIR"
ROOTFS="$WORKSPACE/$PKG_NAME"
CONTAINER="$PKG_NAME-$ARCH"
RELEASE_TAR="$DEPS_DIST_DIR/$PKG_NAME-$PLT-$ARCH.tar.zst"

cleanup() {
    docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
}

build_alpine_rootfs_linux() {
    cd "$WORKSPACE"
    cleanup
    rm -rf "$ROOTFS"
    mkdir -p "$ROOTFS"

    docker run --name="$CONTAINER" "alpine:$ALPINE_VERSION" \
        sh -c "apk add --no-cache bash nftables podman tar util-linux zstd && rm -rf /var/lib/containers"

    docker export "$CONTAINER" | tar -x -C "$ROOTFS"
    install -D -m 0644 "$DEPS_DIR/config/containers.conf" "$ROOTFS/etc/containers/containers.conf"
}

release() {
    cd "$WORKSPACE"
    tar --zstd -cvf "$RELEASE_TAR" -C "$ROOTFS" .
}

trap cleanup EXIT

build_alpine_rootfs_linux
release
