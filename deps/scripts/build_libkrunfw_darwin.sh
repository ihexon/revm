#!/usr/bin/env bash
set -xe
set -o pipefail

# shellcheck source=common.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

PKG_NAME="libkrunfw"
WORKSPACE="$DEPS_WORK_DIR"
LIBKRUNFW_SRC="$WORKSPACE/$PKG_NAME"
PREFIX="$LIBKRUNFW_SRC/_install_"

RELEASE_TAR="$DEPS_DIST_DIR/$PKG_NAME-$PLT-$ARCH.tar.zst"
SRC_ARCHIVE="$DEPS_DIST_DIR/$PKG_NAME-src-Linux-aarch64.tar.zst"

build_libkrunfw_darwin() {
    cd "$WORKSPACE"

    if [[ ! -f "$SRC_ARCHIVE" ]]; then
        echo "prebuilt $SRC_ARCHIVE not found" >&2
        exit 100
    fi

    rm -rf "$LIBKRUNFW_SRC"
    tar --zstd -xf "$SRC_ARCHIVE" -C "$WORKSPACE"

    cd "$LIBKRUNFW_SRC"

    if [[ ! -f kernel.c ]]; then
        echo "kernel.c not found; build libkrunfw on Linux first" >&2
        exit 100
    fi
    # kernel.c must come from the pinned revision and our current configs.
    # Do not label an older intermediate payload with a newer source pin.
    grep -Fx "libkrunfw=$LIBKRUNFW_COMMIT" config-audit/source.txt
    (cd "$DEPS_DIR/config" && shasum -a 256 -c "$LIBKRUNFW_SRC/config-audit/config.sha256")

    make PREFIX="$PREFIX" -j"$DEPS_BUILD_JOBS"
    rm -rf "$PREFIX"
    make PREFIX="$PREFIX" install
    mkdir -p "$PREFIX/share/revm/kernel"
    cp config-audit/* "$PREFIX/share/revm/kernel/"
}

release() {
    tar --zstd -cvf "$RELEASE_TAR" -C "$PREFIX" .
}

build_libkrunfw_darwin
release
