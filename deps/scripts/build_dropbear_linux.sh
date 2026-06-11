#!/usr/bin/env bash
set -xe
set -o pipefail

# shellcheck source=common.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

PKG_NAME="dropbear"
WORKSPACE="$DEPS_WORK_DIR"
SRC_DIR="$WORKSPACE/$PKG_NAME"
PREFIX="$SRC_DIR/_install_"
RELEASE_TAR="$DEPS_DIST_DIR/$PKG_NAME-$PLT-$ARCH.tar.zst"

DROPBEAR_PATCH="$DEPS_DIR/patches/dropbear.diff"
DROPBEAR_PROGRAMS="dropbear dbclient dropbearkey dropbearconvert scp"

build_dropbear_linux() {
    cd "$WORKSPACE"
    rm -rf "$SRC_DIR"
    git clone -b "$DROPBEAR_REF" "$DROPBEAR_REPO" "$SRC_DIR"
    cd "$SRC_DIR"
    git apply "$DROPBEAR_PATCH"

    LDFLAGS="-Wl,--gc-sections" CFLAGS="-ffunction-sections -fdata-sections -DDROPBEAR_LISTEN_BACKLOG=50" bash ./configure \
        --prefix="$PREFIX" \
        --disable-zlib \
        --disable-syslog \
        --disable-wtmpx \
        --disable-pam \
        --disable-utmpx \
        --disable-wtmp \
        --disable-shadow \
        --disable-fuzz \
        --disable-lastlog \
        --enable-static
    make PROGRAMS="$DROPBEAR_PROGRAMS" MULTI=1
    make PROGRAMS="$DROPBEAR_PROGRAMS" MULTI=1 install
}

release() {
    cd "$WORKSPACE"
    tar --zstd -cvf "$RELEASE_TAR" -C "$PREFIX" .
}

build_dropbear_linux
release
