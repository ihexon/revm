#!/usr/bin/env bash
set -xe
set -o pipefail

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
    sed -i '' 's/tag = "0.2.0rc1"/tag = "v0.2.0-rc2"/g' \
        bindings/libkrun-via-cdylib-weak/Cargo.toml \
        bindings/init-blob-via-cdylib/Cargo.toml
    patch_ffier_generator "$LIBKRUN_SRC"
}

build_libkrun_darwin() {
    export RUSTUP_TOOLCHAIN="${RUSTUP_TOOLCHAIN:-nightly}"
    install_rust_linux_musl_target
    brew install lld

    cd "$LIBKRUN_SRC"
    make clean
    TIMESYNC=1 make PREFIX="$PREFIX" BLK=1 NET=1 FFI=1
    verify_libkrun_bundle "$LIBKRUN_SRC/target/release"
    TIMESYNC=1 make PREFIX="$PREFIX" BLK=1 NET=1 FFI=1 install

    rm -rf "$PREFIX/lib/pkgconfig"
    install_name_tool -id "libkrun_init.0.dylib" "$PREFIX/lib/libkrun_init.0.1.0.dylib"
    for dylib in "$PREFIX"/lib/*.dylib; do
        [[ -f "$dylib" ]] || continue
        install_name_tool -add_rpath "@loader_path" "$dylib" 2>/dev/null || true
        codesign --force --sign - "$dylib"
    done
    verify_libkrun_bundle "$PREFIX/lib"
}

release() {
    tar --zstd -cvf "$RELEASE_TAR" -C "$PREFIX" .
}

checkout_libkrun
build_libkrun_darwin
release
