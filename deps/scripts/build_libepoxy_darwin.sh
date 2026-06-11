#!/usr/bin/env bash
set -xe
set -o pipefail

# shellcheck source=common.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

PKG_NAME="libepoxy"
WORKSPACE="$DEPS_WORK_DIR"
PREFIX="$WORKSPACE/$PKG_NAME/_install_"
RELEASE_TAR="$DEPS_DIST_DIR/$PKG_NAME-$PLT-$ARCH.tar.zst"

download() {
    local url="$1"
    local output="$2"

    if [[ ! -f "$output" ]]; then
        curl -fL --retry 3 -o "$output" "$url"
    fi
}

verify_sha256() {
    local expected="$1"
    local file="$2"
    local actual

    actual="$(shasum -a 256 "$file" | awk '{print $1}')"
    if [[ "$actual" != "$expected" ]]; then
        echo "sha256 mismatch for $file: expected $expected, got $actual" >&2
        exit 1
    fi
}

build_libepoxy_darwin() {
    local dist="$WORKSPACE/distfiles/libepoxy-$LIBEPOXY_VERSION.tar.xz"
    local src="$WORKSPACE/build/libepoxy-$LIBEPOXY_VERSION"

    brew install meson ninja pkg-config

    mkdir -p "$WORKSPACE/distfiles" "$WORKSPACE/build"
    download "https://download.gnome.org/sources/libepoxy/1.5/libepoxy-$LIBEPOXY_VERSION.tar.xz" "$dist"
    verify_sha256 "$LIBEPOXY_SHA256" "$dist"

    rm -rf "$src" "$PREFIX"
    tar -xf "$dist" -C "$WORKSPACE/build"

    cd "$src"
    meson setup build-static \
        --prefix="$PREFIX" \
        --libdir=lib \
        --buildtype=release \
        --default-library=static \
        -Dglx=no \
        -Degl=no \
        -Dx11=false \
        -Dtests=false
    meson compile -C build-static
    meson install -C build-static
}

release() {
    cd "$WORKSPACE"
    tar --zstd -cvf "$RELEASE_TAR" -C "$PREFIX" .
}

build_libepoxy_darwin
release
