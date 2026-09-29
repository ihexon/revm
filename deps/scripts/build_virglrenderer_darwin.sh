#!/usr/bin/env bash
set -xe
set -o pipefail

# shellcheck source=common.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

PKG_NAME="virglrenderer"
WORKSPACE="$DEPS_WORK_DIR"
PREFIX="$WORKSPACE/$PKG_NAME/_install_"
LIBEPOXY_PREFIX="$WORKSPACE/libepoxy/_install_"
LIBEPOXY_TAR="$DEPS_DIST_DIR/libepoxy-Darwin-arm64.tar.zst"
RELEASE_TAR="$DEPS_DIST_DIR/$PKG_NAME-$PLT-$ARCH.tar.zst"

MOLTENVK_PREFIX="${MOLTENVK_PREFIX:-}"

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

unpack_libepoxy_darwin() {
    if [[ ! -f "$LIBEPOXY_TAR" ]]; then
        echo "prebuilt $LIBEPOXY_TAR not found" >&2
        exit 100
    fi

    rm -rf "$LIBEPOXY_PREFIX"
    mkdir -p "$LIBEPOXY_PREFIX"
    tar --zstd -xf "$LIBEPOXY_TAR" -C "$LIBEPOXY_PREFIX"
}

build_virglrenderer_darwin() {
    local dist="$WORKSPACE/distfiles/virglrenderer-$VIRGLRENDERER_VERSION.tar.gz"
    local src="$WORKSPACE/build/virglrenderer-$VIRGLRENDERER_VERSION"
    local moltenvk_lib
    local spirv_cross_lib
    local spirv_tools_lib

    brew tap slp/krun
    brew trust slp/krun
    brew install meson ninja pkg-config molten-vk
    brew info molten-vk
    MOLTENVK_PREFIX="${MOLTENVK_PREFIX:-$(brew --prefix molten-vk)}"
    export MOLTENVK_PREFIX

    moltenvk_lib="$MOLTENVK_PREFIX/lib/libMoltenVK.a"
    spirv_cross_lib="$MOLTENVK_PREFIX/libexec/lib/libSPIRVCross.a"
    spirv_tools_lib="$MOLTENVK_PREFIX/libexec/lib/libSPIRVTools.a"

    if [[ ! -f "$moltenvk_lib" || ! -f "$spirv_cross_lib" || ! -f "$spirv_tools_lib" ]]; then
        echo "MoltenVK static libraries were not found; install Homebrew molten-vk first." >&2
        exit 1
    fi

    unpack_libepoxy_darwin

    mkdir -p "$WORKSPACE/distfiles" "$WORKSPACE/build"
    download "https://gitlab.freedesktop.org/slp/virglrenderer/-/archive/$VIRGLRENDERER_VERSION/virglrenderer-$VIRGLRENDERER_VERSION.tar.gz" "$dist"
    verify_sha256 "$VIRGLRENDERER_SHA256" "$dist"

    rm -rf "$src" "$PREFIX"
    tar -xf "$dist" -C "$WORKSPACE/build"

    cd "$src"
    patch -p1 < "$REPO_ROOT/deps/patches/virglrenderer-resource-map-fixed.patch"
    # macOS has no Linux memfd seals. Keep the render-server metadata size
    # validation, but omit the seal operations so Venus can run in-process.
    patch -p1 < "$REPO_ROOT/deps/patches/virglrenderer-darwin-render-server.patch"
    # Darwin does not define MSG_CMSG_CLOEXEC; upstream virglrenderer uses a
    # zero fallback for its Unix socket transport on this platform.
    patch -p1 < "$REPO_ROOT/deps/patches/virglrenderer-darwin-socket.patch"
    patch -p1 < "$REPO_ROOT/deps/patches/virglrenderer-darwin-render-socket.patch"
    perl -0pi -e "s|add_project_link_arguments\\('-lMoltenVK', language : 'c'\\)|add_project_link_arguments('$moltenvk_lib', '$spirv_cross_lib', '$spirv_tools_lib', language : 'c')|" meson.build
    perl -0pi -e "s|-I/opt/homebrew/opt/molten-vk/libexec/include|-I$MOLTENVK_PREFIX/libexec/include|" meson.build
    perl -0pi -e "s|if not with_host_windows\\n   subdir\\('vtest'\\)\\nendif\\n\\n||" meson.build

    PKG_CONFIG_PATH="$LIBEPOXY_PREFIX/lib/pkgconfig" \
    PKG_CONFIG_ALL_STATIC=1 \
    CPPFLAGS="-I$LIBEPOXY_PREFIX/include -I$MOLTENVK_PREFIX/libexec/include" \
    LDFLAGS="-L$LIBEPOXY_PREFIX/lib" \
        meson setup build-static \
            --prefix="$PREFIX" \
            --libdir=lib \
            --buildtype=release \
            --default-library=static \
            -Dvenus=true \
            -Drender-server=true \
            -Ddrm=disabled \
            '-Dplatforms=[]'

    PKG_CONFIG_PATH="$LIBEPOXY_PREFIX/lib/pkgconfig" \
    PKG_CONFIG_ALL_STATIC=1 \
        meson compile -C build-static
    meson install -C build-static

    cat >> "$PREFIX/lib/pkgconfig/virglrenderer.pc" <<EOF
Libs.private: $moltenvk_lib $spirv_cross_lib $spirv_tools_lib -framework Metal -framework Foundation -framework QuartzCore -framework CoreGraphics -framework IOSurface -framework IOKit -framework AppKit -lc++ -lobjc
EOF
    # The krunkit fork contains the virgl APIs required by current rutabaga,
    # but retains the historical 0.10.x Meson project version. Current
    # libkrun checks the pkg-config compatibility version at build time.
    sed -i '' -E 's/^Version: .*/Version: 1.3.0/' "$PREFIX/lib/pkgconfig/virglrenderer.pc"
}

release() {
    cd "$WORKSPACE"
    tar --zstd -cvf "$RELEASE_TAR" -C "$PREFIX" .
}

build_virglrenderer_darwin
release
