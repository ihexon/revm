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
    checkout_source "$LIBKRUNFW_REPO" "$LIBKRUNFW_COMMIT" "$LIBKRUNFW_SRC"
    cd "$LIBKRUNFW_SRC"

    local config_arch="$ARCH" kernel_arch="$ARCH"
    if [[ "$ARCH" == "aarch64" ]]; then
        kernel_arch=arm64
    fi
    export ARCH="$kernel_arch"
    local audit="$LIBKRUNFW_SRC/config-audit"
    mkdir -p "$audit"
    cp "config-libkrunfw_$config_arch" "$audit/upstream.config"

    cp -av "$DEPS_DIR/config/config-libkrunfw_aarch64" "$LIBKRUNFW_SRC/config-libkrunfw_aarch64"
    cp -av "$DEPS_DIR/config/config-libkrunfw_x86_64" "$LIBKRUNFW_SRC/config-libkrunfw_x86_64"

    cp "config-libkrunfw_$config_arch" "$audit/revm.config"
    local kernel
    kernel="$(sed -n 's/^KERNEL_VERSION = //p' Makefile)"
    [[ "$kernel" == linux-* ]] || { echo 'Cannot determine kernel version' >&2; exit 1; }
    # Use upstream's patching and Linux's Kconfig resolver for both inputs.
    make "$kernel"
    make -C "$kernel" KCONFIG_CONFIG="$audit/upstream.config" olddefconfig
    make -C "$kernel" olddefconfig
    cp "$kernel/.config" "$audit/final.config"
    "$DEPS_DIR/scripts/check_kernel_config.sh" "$LIBKRUNFW_SRC/$kernel" "$audit" "$config_arch"
    printf 'libkrunfw=%s\nkernel=%s\n' "$LIBKRUNFW_COMMIT" "$kernel" > "$audit/source.txt"
    (cd "$DEPS_DIR/config" && sha256sum config-libkrunfw_aarch64 config-libkrunfw_x86_64) > "$audit/config.sha256"

    make PREFIX="$PREFIX" -j"$DEPS_BUILD_JOBS"
    rm -rf "$PREFIX"
    make PREFIX="$PREFIX" LIBDIR_Linux=lib64 install
    mkdir -p "$PREFIX/share/revm/kernel"
    cp "$audit/"* "$PREFIX/share/revm/kernel/"
}

repack_libkrunfw_source() {
    cd "$WORKSPACE"
    # Darwin only consumes the generated kernel.c, build scripts and audit.
    tar --zstd --exclude="$PKG_NAME/linux-*" --exclude="$PKG_NAME/.git" \
        --exclude="$PKG_NAME/_install_" --exclude="$PKG_NAME/tarballs" \
        -cf "$SRC_ARCHIVE" -C "$WORKSPACE" "$PKG_NAME"
}

release() {
    tar --zstd -cvf "$RELEASE_TAR" -C "$PREFIX" .
}

build_libkrunfw_linux
repack_libkrunfw_source
release
