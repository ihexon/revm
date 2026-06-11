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

set_libkrun_crate_type() {
    cd "$LIBKRUN_SRC"
    local crate_type="$1"
    perl -0pi -e "s/crate-type = \\[[^\\]]+\\]/crate-type = [$crate_type]/" src/libkrun/Cargo.toml
}

build_libkrun_linux() {
    export RUSTFLAGS="${RUSTFLAGS:-} -C linker=gcc -C link-arg=-static-libgcc"

    cd "$LIBKRUN_SRC"
    set_libkrun_crate_type '"cdylib", "staticlib", "lib"'
    make clean
    make PREFIX="$PREFIX" BLK=1 NET=1

    rm -rf "$PREFIX"
    make PREFIX="$PREFIX" BLK=1 NET=1 install
    install -m 644 target/release/libkrun.a "$PREFIX/lib64/"
}

release() {
    tar --zstd -cvf "$RELEASE_TAR" -C "$PREFIX" .
}

checkout_libkrun
build_libkrun_linux
release
