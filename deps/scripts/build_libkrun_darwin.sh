#!/usr/bin/env bash
set -xe
set -o pipefail

# shellcheck source=common.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

PKG_NAME="libkrun"
WORKSPACE="$DEPS_WORK_DIR"
LIBKRUN_SRC="$WORKSPACE/$PKG_NAME"
PREFIX="$LIBKRUN_SRC/_install_"
LIBEPOXY_PREFIX="$WORKSPACE/libepoxy/_install_"
VIRGLRENDERER_PREFIX="$WORKSPACE/virglrenderer/_install_"
LIBEPOXY_TAR="$DEPS_DIST_DIR/libepoxy-Darwin-arm64.tar.zst"
VIRGLRENDERER_TAR="$DEPS_DIST_DIR/virglrenderer-Darwin-arm64.tar.zst"
RELEASE_TAR="$DEPS_DIST_DIR/$PKG_NAME-$PLT-$ARCH.tar.zst"

MOLTENVK_PREFIX="${MOLTENVK_PREFIX:-}"
RUTABAGA_VERSION="0.1.85"
RUTABAGA_TAR="$DEPS_DIST_DIR/rutabaga_gfx-$RUTABAGA_VERSION.crate"

checkout_libkrun() {
    rm -rf "$LIBKRUN_SRC"
    git clone "$LIBKRUN_REPO" "$LIBKRUN_SRC"
    cd "$LIBKRUN_SRC" && git checkout "$LIBKRUN_COMMIT"
    # See the Linux builder for why the weak-client binding crates use ffier rc2.
    sed -i '' 's/tag = "0.2.0rc1"/tag = "v0.2.0-rc2"/g' \
        bindings/libkrun-via-cdylib-weak/Cargo.toml \
        bindings/init-blob-via-cdylib/Cargo.toml
    patch_ffier_generator "$LIBKRUN_SRC"
}

patch_rutabaga_macos_venus() {
    local vendor_dir="$LIBKRUN_SRC/third_party/rutabaga_gfx"
    local crate_root="$vendor_dir/rutabaga_gfx-$RUTABAGA_VERSION"

    # libkrun 2.0 still consumes rutabaga 0.1.85. Its macOS Venus resource
    # export predates virglrenderer's pointer-backed Apple blob type, so keep a
    # small local patch until the matching rutabaga release is available.
    if [[ ! -f "$RUTABAGA_TAR" ]]; then
        curl -fL --retry 3 -o "$RUTABAGA_TAR" \
            "https://crates.io/api/v1/crates/rutabaga_gfx/$RUTABAGA_VERSION/download"
    fi
    rm -rf "$vendor_dir"
    mkdir -p "$vendor_dir"
    tar -xf "$RUTABAGA_TAR" -C "$vendor_dir"
    patch -p1 -d "$crate_root" < "$REPO_ROOT/deps/patches/rutabaga-macos-venus-map.patch"
    mkdir -p "$LIBKRUN_SRC/third_party"
    cat >> "$LIBKRUN_SRC/Cargo.toml" <<'EOF'

[patch.crates-io]
rutabaga_gfx = { path = "third_party/rutabaga_gfx/rutabaga_gfx-0.1.85" }
EOF
    patch -p1 -d "$LIBKRUN_SRC" < "$REPO_ROOT/deps/patches/libkrun-macos-venus-map.patch"
}

unpack_gpu_deps_darwin() {
    if [[ ! -f "$LIBEPOXY_TAR" ]]; then
        echo "prebuilt $LIBEPOXY_TAR not found" >&2
        exit 100
    fi

    if [[ ! -f "$VIRGLRENDERER_TAR" ]]; then
        echo "prebuilt $VIRGLRENDERER_TAR not found" >&2
        exit 100
    fi

    rm -rf "$LIBEPOXY_PREFIX" "$VIRGLRENDERER_PREFIX"
    mkdir -p "$LIBEPOXY_PREFIX" "$VIRGLRENDERER_PREFIX"
    tar --zstd -xf "$LIBEPOXY_TAR" -C "$LIBEPOXY_PREFIX"
    tar --zstd -xf "$VIRGLRENDERER_TAR" -C "$VIRGLRENDERER_PREFIX"
}

install_build_deps_darwin() {
    brew tap slp/krun
    brew trust slp/krun
    brew install pkg-config molten-vk lld
    brew info molten-vk
    MOLTENVK_PREFIX="${MOLTENVK_PREFIX:-$(brew --prefix molten-vk)}"
    export MOLTENVK_PREFIX

    unpack_gpu_deps_darwin
}

build_libkrun_darwin() {
    export RUSTUP_TOOLCHAIN="${RUSTUP_TOOLCHAIN:-nightly}"
    install_rust_linux_musl_target

    install_build_deps_darwin

    export PKG_CONFIG_PATH="$VIRGLRENDERER_PREFIX/lib/pkgconfig:$LIBEPOXY_PREFIX/lib/pkgconfig${PKG_CONFIG_PATH:+:$PKG_CONFIG_PATH}"
    export LIBRARY_PATH="$VIRGLRENDERER_PREFIX/lib:$LIBEPOXY_PREFIX/lib:$MOLTENVK_PREFIX/lib:$MOLTENVK_PREFIX/libexec/lib${LIBRARY_PATH:+:$LIBRARY_PATH}"
    export CPATH="$VIRGLRENDERER_PREFIX/include:$LIBEPOXY_PREFIX/include:$MOLTENVK_PREFIX/libexec/include${CPATH:+:$CPATH}"

    cd "$LIBKRUN_SRC"
    patch_rutabaga_macos_venus
    make clean
    TIMESYNC=1 make PREFIX="$PREFIX" BLK=1 NET=1 GPU=1 FFI=1
    verify_libkrun_bundle "$LIBKRUN_SRC/target/release"
    TIMESYNC=1 make PREFIX="$PREFIX" BLK=1 NET=1 GPU=1 FFI=1 install

    rm -rf "$PREFIX/lib/pkgconfig"
    # krun-init embeds the temporary GitHub Actions checkout in its Mach-O
    # install name. Normalize it before publishing the dependency archive so
    # consumers can relocate the bundled library.
    install_name_tool -id "libkrun_init.0.dylib" "$PREFIX/lib/libkrun_init.0.1.0.dylib"
    verify_libkrun_bundle "$PREFIX/lib"
    # Homebrew's GPU stack is shared. Carry the transitive dylibs in the
    # revm bundle and rewrite their install names below so the bundle does
    # not depend on /opt/homebrew at runtime.
    gpu_dylibs=()
    for dep in "$VIRGLRENDERER_PREFIX"/lib/*.dylib \
               "$LIBEPOXY_PREFIX"/lib/*.dylib \
               "$MOLTENVK_PREFIX"/lib/libMoltenVK.dylib; do
        [[ -e "$dep" ]] || continue
        dest="$PREFIX/lib/$(basename "$dep")"
        install -m 755 "$dep" "$dest"
        gpu_dylibs+=("$dest")
    done

    for dylib in "${gpu_dylibs[@]}"; do
        install_name_tool -id "@rpath/$(basename "$dylib")" "$dylib"
    done

    for dylib in "$PREFIX"/lib/*.dylib; do
        [[ -f "$dylib" ]] || continue
        install_name_tool -add_rpath "@loader_path" "$dylib" 2>/dev/null || true
        install_name_tool -change "$MOLTENVK_PREFIX/lib/libMoltenVK.dylib" \
            "@rpath/libMoltenVK.dylib" "$dylib" 2>/dev/null || true
        install_name_tool -change "$LIBEPOXY_PREFIX/lib/libepoxy.0.dylib" \
            "@rpath/libepoxy.0.dylib" "$dylib" 2>/dev/null || true
        for dep in "$VIRGLRENDERER_PREFIX"/lib/libvirglrenderer*.dylib \
                   "$LIBEPOXY_PREFIX"/lib/libepoxy*.dylib; do
            [[ -e "$dep" ]] || continue
            install_name_tool -change "$dep" "@rpath/$(basename "$dep")" "$dylib" 2>/dev/null || true
        done
        # install_name_tool invalidates the linker signature. macOS may kill
        # an unsigned or stale-signed dylib before main() when it is loaded by
        # the signed revm executable, so refresh an ad-hoc signature after all
        # install-name and rpath changes are complete.
        codesign --force --sign - "$dylib"
    done

    for dylib in "$PREFIX"/lib/*.dylib; do
        [[ -f "$dylib" ]] || continue
        if otool -L "$dylib" | grep -E '/opt/homebrew|/tmp/.deps' >/dev/null; then
            echo "non-relocatable dependency in $dylib" >&2
            otool -L "$dylib" >&2
            exit 1
        fi
    done
}

release() {
    tar --zstd -cvf "$RELEASE_TAR" -C "$PREFIX" .
}

checkout_libkrun
build_libkrun_darwin
release
