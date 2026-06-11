#!/usr/bin/env bash
set -xe
set -o pipefail

# shellcheck source=common.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

PKG_NAME="busybox"
WORKSPACE="$DEPS_WORK_DIR"
SRC_DIR="$WORKSPACE/$PKG_NAME"
PREFIX="$SRC_DIR/_install_"
RELEASE_TAR="$DEPS_DIST_DIR/$PKG_NAME-$PLT-$ARCH.tar.zst"

case "$ARCH" in
    aarch64)
        BUSYBOX_DEB_URL="$BUSYBOX_AARCH64_DEB_URL"
        ;;
    x86_64)
        BUSYBOX_DEB_URL="$BUSYBOX_X86_64_DEB_URL"
        ;;
    *)
        echo "unsupported arch: $ARCH" >&2
        exit 1
        ;;
esac

build_busybox_linux() {
    rm -rf "$SRC_DIR"
    mkdir -p "$SRC_DIR"
    cd "$SRC_DIR"
    wget "$BUSYBOX_DEB_URL" --output-document=busybox.deb
    dpkg -X busybox.deb "$PREFIX"
}

release() {
    cd "$WORKSPACE"
    tar --zstd -cvf "$RELEASE_TAR" -C "$PREFIX" .
}

build_busybox_linux
release
