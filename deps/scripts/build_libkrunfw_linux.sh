#!/usr/bin/env bash
set -xe
set -o pipefail

# shellcheck source=common.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

PKG_NAME="libkrunfw"
WORKSPACE="$DEPS_WORK_DIR"
LIBKRUNFW_SRC="$WORKSPACE/$PKG_NAME"
PREFIX="$LIBKRUNFW_SRC/_install_"

SRC_ARCHIVE="$DEPS_DIST_DIR/$PKG_NAME-src-$PLT-$ARCH.tar.zst"
RELEASE_TAR="$DEPS_DIST_DIR/$PKG_NAME-$PLT-$ARCH.tar.zst"

build_libkrunfw_linux() {
    rm -rf "$LIBKRUNFW_SRC"
    git clone "$LIBKRUNFW_REPO" "$LIBKRUNFW_SRC"
    cd "$LIBKRUNFW_SRC" && git checkout "$LIBKRUNFW_COMMIT"

    cp -av "$DEPS_DIR/config/config-libkrunfw_aarch64" "$LIBKRUNFW_SRC/config-libkrunfw_aarch64"
    cp -av "$DEPS_DIR/config/config-libkrunfw_x86_64" "$LIBKRUNFW_SRC/config-libkrunfw_x86_64"

    if [[ "$ARCH" == "aarch64" ]]; then
        ARCH=arm64 make PREFIX="$PREFIX" -j8
        rm -rf "$PREFIX"
        ARCH=arm64 make PREFIX="$PREFIX" -j8 install
    else
        make PREFIX="$PREFIX" -j8
        rm -rf "$PREFIX"
        make PREFIX="$PREFIX" -j8 install
    fi
}

repack_libkrunfw_source() {
    cd "$WORKSPACE"
    tar --zstd -cf "$SRC_ARCHIVE" -C "$(dirname "$LIBKRUNFW_SRC")" "$(basename "$LIBKRUNFW_SRC")"
}

release() {
    tar --zstd -cvf "$RELEASE_TAR" -C "$PREFIX" .
}

build_libkrunfw_linux
repack_libkrunfw_source
release
