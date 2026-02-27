#!/usr/bin/env bash
set -euo pipefail

readonly ASSETS_BASE="https://github.com/ihexon/revm-assets/releases/download/v2.0.4"
readonly HOMEBREW_PREFIX="${HOMEBREW_PREFIX:-/opt/homebrew}"

readonly RED="\033[31m" GREEN="\033[32m" RESET="\033[0m"

log_info() { echo -e "${GREEN}[INFO]${RESET} $*"; }
log_err() {
    echo -e "${RED}[ERROR]${RESET} $*" >&2
    exit 1
}

fetch_asset() {
    local name="$1" url="$2" dest="$3"
    log_info "Fetching $name..."
    mkdir -p "$dest" && wget -qO- "$url" | bsdtar --zstd -x -C "$dest"
}

restore_placeholders() {
    local static="$1"
    : >"$static/rootfs/rootfs.tar.zst"
}

build() {
    local dirty=0
    for arg in "$@"; do [[ "$arg" == "--dirty" ]] && dirty=1; done

    local os arch
    os="$(uname -s)"   # Darwin | Linux
    arch="$(uname -m)" # arm64  | aarch64

    local workspace
    workspace="$(cd "$(dirname "$0")" && pwd)"
    local outdir="$workspace/out"
    local bindir="$outdir/bin"
    local libdir="$outdir/lib"
    local helperdir="$outdir/helper"
    local depsdir="/tmp/.deps"
    local static="$workspace/pkg/static_resources"

    rm -rf "$outdir"
    mkdir -p "$bindir" "$libdir" "$helperdir"

    # ── 1. Build guest-agent ────────────────────────────────────────────────
    log_info "Building guest-agent..."
    (cd "$workspace/guest-agent" &&
        CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
            -ldflags="-s -w" \
            -o "$helperdir/guest-agent" main.go)

    # ── 2. Fetch dependencies ───────────────────────────────────────────────
    local asset_os asset_arch
    if [[ "$os" == "Darwin" ]]; then
        asset_os="Darwin"
        asset_arch="arm64"
    else
        asset_os="Linux"
        asset_arch="aarch64"
    fi

    if [[ "$dirty" == 1 && -d "$depsdir" ]]; then
        log_info "Dirty mode: reusing cached deps in $depsdir"
    else
        [[ "$dirty" == 1 ]] && log_info "Dirty mode: $depsdir not found, downloading anyway..."
        rm -rf "$depsdir"
        mkdir -p "$depsdir"
        fetch_asset "libkrun"   "$ASSETS_BASE/libkrun-${asset_os}-${asset_arch}.tar.zst"   "$depsdir/libkrun"
        fetch_asset "libkrunfw" "$ASSETS_BASE/libkrunfw-${asset_os}-${asset_arch}.tar.zst" "$depsdir/libkrunfw"
    fi

    log_info "Fetching rootfs..."
    wget -qO "$static/rootfs/rootfs.tar.zst" "$ASSETS_BASE/alpine-rootfs-Linux-aarch64.tar.zst"

    # ── 3. Prepare shared libraries ─────────────────────────────────────────
    log_info "Preparing shared libraries..."
    if [[ "$os" == "Darwin" ]]; then
        cp -av "$depsdir/libkrun/lib/"*.dylib "$depsdir/libkrunfw/lib/"*.dylib "$libdir/"
        cp -av "$HOMEBREW_PREFIX/opt/libepoxy/lib/libepoxy.0.dylib"             "$libdir/"
        cp -av "$HOMEBREW_PREFIX/opt/virglrenderer/lib/libvirglrenderer.1.dylib" "$libdir/"
        cp -av "$HOMEBREW_PREFIX/opt/molten-vk/lib/libMoltenVK.dylib"           "$libdir/"
        rm -rf "$libdir/pkgconfig"

        # Rewrite Homebrew absolute paths in libkrun.1.dylib
        install_name_tool -change "$HOMEBREW_PREFIX/opt/libepoxy/lib/libepoxy.0.dylib"             "@loader_path/libepoxy.0.dylib"             "$libdir/libkrun.1.dylib" 2>/dev/null || true
        install_name_tool -change "$HOMEBREW_PREFIX/opt/virglrenderer/lib/libvirglrenderer.1.dylib" "@loader_path/libvirglrenderer.1.dylib"     "$libdir/libkrun.1.dylib" 2>/dev/null || true
        install_name_tool -change "$HOMEBREW_PREFIX/opt/molten-vk/lib/libMoltenVK.dylib"           "@loader_path/libMoltenVK.dylib"             "$libdir/libkrun.1.dylib" 2>/dev/null || true

        # Rewrite Homebrew absolute paths in libkrunfw.5.dylib
        install_name_tool -change "$HOMEBREW_PREFIX/opt/libepoxy/lib/libepoxy.0.dylib"             "@loader_path/libepoxy.0.dylib"             "$libdir/libkrunfw.5.dylib" 2>/dev/null || true
        install_name_tool -change "$HOMEBREW_PREFIX/opt/virglrenderer/lib/libvirglrenderer.1.dylib" "@loader_path/libvirglrenderer.1.dylib"     "$libdir/libkrunfw.5.dylib" 2>/dev/null || true
        install_name_tool -change "$HOMEBREW_PREFIX/opt/molten-vk/lib/libMoltenVK.dylib"           "@loader_path/libMoltenVK.dylib"             "$libdir/libkrunfw.5.dylib" 2>/dev/null || true

        # Rewrite virglrenderer's own dependencies
        install_name_tool -change "$HOMEBREW_PREFIX/opt/libepoxy/lib/libepoxy.0.dylib"             "@loader_path/libepoxy.0.dylib"             "$libdir/libvirglrenderer.1.dylib" 2>/dev/null || true
        install_name_tool -change "$HOMEBREW_PREFIX/opt/molten-vk/lib/libMoltenVK.dylib"           "@loader_path/libMoltenVK.dylib"             "$libdir/libvirglrenderer.1.dylib" 2>/dev/null || true

        # Fix install names and re-sign Homebrew dylibs
        install_name_tool -id "@loader_path/libepoxy.0.dylib"             "$libdir/libepoxy.0.dylib"
        codesign --force -s - "$libdir/libepoxy.0.dylib"
        install_name_tool -id "@loader_path/libvirglrenderer.1.dylib"     "$libdir/libvirglrenderer.1.dylib"
        codesign --force -s - "$libdir/libvirglrenderer.1.dylib"
        install_name_tool -id "@loader_path/libMoltenVK.dylib"            "$libdir/libMoltenVK.dylib"
        codesign --force -s - "$libdir/libMoltenVK.dylib"

        # Use pkg-config files provided by Homebrew.
        local pkgcfgdir
        pkgcfgdir="$(brew --prefix libarchive)/lib/pkgconfig:$(brew --prefix e2fsprogs)/lib/pkgconfig"

        PKG_CONFIG_PATH="$pkgcfgdir" golangci-lint run
    else
        # lib64 -> lib symlinks so CGo header search works
        [[ -d "$depsdir/libkrun/lib64" && ! -e "$depsdir/libkrun/lib" ]] && ln -s lib64 "$depsdir/libkrun/lib"
        [[ -d "$depsdir/libkrunfw/lib64" && ! -e "$depsdir/libkrunfw/lib" ]] && ln -s lib64 "$depsdir/libkrunfw/lib"

        cp -av "$depsdir/libkrun/lib64/"*.so* "$libdir/"
        cp -av "$depsdir/libkrunfw/lib64/"*.so* "$libdir/"
    fi

    # ── 4. Build revm ───────────────────────────────────────────────────────
    local version commit
    version="$(git -C "$workspace" describe --tags --abbrev=0 2>/dev/null || echo "unknown")"
    commit="$(git -C "$workspace" rev-parse --short HEAD 2>/dev/null || echo "unknown")"
    log_info "Building revm ($version-$commit)..."

    local ldflags="-X linuxvm/pkg/define.Version=$version -X linuxvm/pkg/define.CommitID=$commit"

    if [[ "$os" == "Darwin" ]]; then
        PKG_CONFIG_PATH="$pkgcfgdir" \
            go build -ldflags="$ldflags" -o "$bindir/revm" "$workspace/cmd/revm"

        # Fix revm's dylib references (revm is in bin/, dylibs are in lib/)
        install_name_tool -change "libkrun.1.dylib" "@loader_path/../lib/libkrun.1.dylib" "$bindir/revm"
        install_name_tool -change "libkrunfw.5.dylib" "@loader_path/../lib/libkrunfw.5.dylib" "$bindir/revm"
        codesign --entitlements "$workspace/revm.entitlements" --force -s - "$bindir/revm"
    else
        # libarchive and e2fsprogs are provided by the system; find via pkg-config.
        CGO_ENABLED=1 \
            go build -ldflags="$ldflags" -o "$bindir/revm" "$workspace/cmd/revm"

        patchelf --set-rpath '$ORIGIN/../lib' "$bindir/revm"
        for sofile in "$libdir"/libkrun*.so.*.*; do
            patchelf --set-rpath '$ORIGIN' "$sofile"
        done
    fi

    # ── 5. Package ──────────────────────────────────────────────────────────
    log_info "Packaging..."
    bsdtar --zstd -cf "$workspace/revm-${os}-${arch}.tar.zst" -C "$outdir" .

    restore_placeholders "$static"
    log_info "Build complete: $workspace/revm-${os}-${arch}.tar.zst"
}

build "$@"
