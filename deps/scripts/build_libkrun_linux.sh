#!/usr/bin/env bash
set -xe
set -o pipefail

# shellcheck source=common.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

PKG_NAME="libkrun"
WORKSPACE="$DEPS_WORK_DIR"
LIBKRUN_SRC="$WORKSPACE/$PKG_NAME"
PREFIX="$LIBKRUN_SRC/_install_"
RELEASE_TAR="$DEPS_DIST_DIR/$PKG_NAME-$PLT-$ARCH.tar.zst"

checkout_libkrun() {
    rm -rf "$LIBKRUN_SRC"
    git clone "$LIBKRUN_REPO" "$LIBKRUN_SRC"
    cd "$LIBKRUN_SRC" && git checkout "$LIBKRUN_COMMIT"
}

build_libkrun_linux() {
    install_rust_linux_musl_target

    export RUSTFLAGS="${RUSTFLAGS:-} -C linker=gcc -C link-arg=-static-libgcc"

    cd "$LIBKRUN_SRC"
    make clean
    make PREFIX="$PREFIX" BLK=1 NET=1
    verify_libkrun_bundle "$LIBKRUN_SRC/target/release"

    rm -rf "$PREFIX"
    make PREFIX="$PREFIX" BLK=1 NET=1 install
    verify_libkrun_bundle "$PREFIX/lib64"
}

release() {
    tar --zstd -cvf "$RELEASE_TAR" -C "$PREFIX" .
}

checkout_libkrun
build_libkrun_linux
release
