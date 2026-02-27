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
    : >"$static/guest-agent/guest-agent"
    : >"$static/rootfs/rootfs.tar.zst"
}

build() {
    local os arch
    os="$(uname -s)"   # Darwin | Linux
    arch="$(uname -m)" # arm64  | aarch64

    local workspace
    workspace="$(cd "$(dirname "$0")" && pwd)"
    local outdir="$workspace/out"
    local bindir="$outdir/bin"
    local depsdir="$outdir/.deps"
    local static="$workspace/pkg/static_resources"

    rm -rf "$outdir"
    mkdir -p "$bindir" "$depsdir"

    # ── 1. Build guest-agent ────────────────────────────────────────────────
    log_info "Building guest-agent..."
    (cd "$workspace/guest-agent" &&
        CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
            -ldflags="-s -w" \
            -o "$static/guest-agent/guest-agent" main.go)

    # ── 2. Fetch dependencies ───────────────────────────────────────────────
    local asset_os asset_arch
    if [[ "$os" == "Darwin" ]]; then
        asset_os="Darwin"
        asset_arch="arm64"
    else
        asset_os="Linux"
        asset_arch="aarch64"
    fi

    fetch_asset "e2fsprogs" "$ASSETS_BASE/e2fsprogs-${asset_os}-${asset_arch}.tar.zst" "$depsdir/e2fsprogs"
    fetch_asset "libkrun" "$ASSETS_BASE/libkrun-${asset_os}-${asset_arch}.tar.zst" "$depsdir/libkrun"
    fetch_asset "libkrunfw" "$ASSETS_BASE/libkrunfw-${asset_os}-${asset_arch}.tar.zst" "$depsdir/libkrunfw"

    log_info "Fetching rootfs..."
    wget -qO "$static/rootfs/rootfs.tar.zst" "$ASSETS_BASE/alpine-rootfs-Linux-aarch64.tar.zst"

    # ── 3. Prepare shared libraries ─────────────────────────────────────────
    log_info "Preparing shared libraries..."
    if [[ "$os" == "Darwin" ]]; then
        cp -av "$depsdir/libkrun/lib/"*.dylib "$depsdir/libkrunfw/lib/"*.dylib "$bindir/"
        cp -av "$HOMEBREW_PREFIX/opt/libepoxy/lib/libepoxy.0.dylib"             "$bindir/"
        cp -av "$HOMEBREW_PREFIX/opt/virglrenderer/lib/libvirglrenderer.1.dylib" "$bindir/"
        cp -av "$HOMEBREW_PREFIX/opt/molten-vk/lib/libMoltenVK.dylib"           "$bindir/"
        rm -rf "$bindir/pkgconfig"

        # Rewrite Homebrew absolute paths in libkrun.1.dylib
        install_name_tool -change "$HOMEBREW_PREFIX/opt/libepoxy/lib/libepoxy.0.dylib"             "@loader_path/libepoxy.0.dylib"             "$bindir/libkrun.1.dylib" 2>/dev/null || true
        install_name_tool -change "$HOMEBREW_PREFIX/opt/virglrenderer/lib/libvirglrenderer.1.dylib" "@loader_path/libvirglrenderer.1.dylib"     "$bindir/libkrun.1.dylib" 2>/dev/null || true
        install_name_tool -change "$HOMEBREW_PREFIX/opt/molten-vk/lib/libMoltenVK.dylib"           "@loader_path/libMoltenVK.dylib"             "$bindir/libkrun.1.dylib" 2>/dev/null || true

        # Rewrite Homebrew absolute paths in libkrunfw.5.dylib
        install_name_tool -change "$HOMEBREW_PREFIX/opt/libepoxy/lib/libepoxy.0.dylib"             "@loader_path/libepoxy.0.dylib"             "$bindir/libkrunfw.5.dylib" 2>/dev/null || true
        install_name_tool -change "$HOMEBREW_PREFIX/opt/virglrenderer/lib/libvirglrenderer.1.dylib" "@loader_path/libvirglrenderer.1.dylib"     "$bindir/libkrunfw.5.dylib" 2>/dev/null || true
        install_name_tool -change "$HOMEBREW_PREFIX/opt/molten-vk/lib/libMoltenVK.dylib"           "@loader_path/libMoltenVK.dylib"             "$bindir/libkrunfw.5.dylib" 2>/dev/null || true

        # Rewrite virglrenderer's own dependencies
        install_name_tool -change "$HOMEBREW_PREFIX/opt/libepoxy/lib/libepoxy.0.dylib"             "@loader_path/libepoxy.0.dylib"             "$bindir/libvirglrenderer.1.dylib" 2>/dev/null || true
        install_name_tool -change "$HOMEBREW_PREFIX/opt/molten-vk/lib/libMoltenVK.dylib"           "@loader_path/libMoltenVK.dylib"             "$bindir/libvirglrenderer.1.dylib" 2>/dev/null || true

        # Fix install names and re-sign Homebrew dylibs
        install_name_tool -id "@loader_path/libepoxy.0.dylib"             "$bindir/libepoxy.0.dylib"
        codesign --force -s - "$bindir/libepoxy.0.dylib"
        install_name_tool -id "@loader_path/libvirglrenderer.1.dylib"     "$bindir/libvirglrenderer.1.dylib"
        codesign --force -s - "$bindir/libvirglrenderer.1.dylib"
        install_name_tool -id "@loader_path/libMoltenVK.dylib"            "$bindir/libMoltenVK.dylib"
        codesign --force -s - "$bindir/libMoltenVK.dylib"

        golangci-lint run
    else
        # lib64 -> lib symlinks so CGo header search works
        [[ -d "$depsdir/libkrun/lib64" && ! -e "$depsdir/libkrun/lib" ]] && ln -s lib64 "$depsdir/libkrun/lib"
        [[ -d "$depsdir/libkrunfw/lib64" && ! -e "$depsdir/libkrunfw/lib" ]] && ln -s lib64 "$depsdir/libkrunfw/lib"

        cp -av "$depsdir/libkrun/lib64/"*.so* "$bindir/"
        cp -av "$depsdir/libkrunfw/lib64/"*.so* "$bindir/"
    fi

    # ── 4. Build revm ───────────────────────────────────────────────────────
    local version commit
    version="$(git -C "$workspace" describe --tags --abbrev=0 2>/dev/null || echo "unknown")"
    commit="$(git -C "$workspace" rev-parse --short HEAD 2>/dev/null || echo "unknown")"
    log_info "Building revm ($version-$commit)..."

    local ldflags="-X linuxvm/pkg/define.Version=$version -X linuxvm/pkg/define.CommitID=$commit"

    if [[ "$os" == "Darwin" ]]; then
        go build -ldflags="$ldflags" -o "$bindir/revm" "$workspace/cmd/revm"

        # Fix revm's dylib references and sign
        install_name_tool -change "libkrun.1.dylib" "@loader_path/libkrun.1.dylib" "$bindir/revm"
        install_name_tool -change "libkrunfw.5.dylib" "@loader_path/libkrunfw.5.dylib" "$bindir/revm"
        codesign --entitlements "$workspace/revm.entitlements" --force -s - "$bindir/revm"
    else
        # libarchive-go lacks cgo_linux_arm64.go; -luuid resolves libblkid.a -> libuuid.a circular dep
        CGO_ENABLED=1 CGO_LDFLAGS="-larchive -luuid" \
            go build -ldflags="$ldflags" -o "$bindir/revm" "$workspace/cmd/revm"

        patchelf --set-rpath '$ORIGIN' "$bindir/revm"
        for sofile in "$bindir"/libkrun*.so.*.*; do
            patchelf --set-rpath '$ORIGIN' "$sofile"
        done
    fi

    # ── 5. Package ──────────────────────────────────────────────────────────
    rm -rf "$depsdir"
    log_info "Packaging..."
    bsdtar --zstd -cf "$workspace/revm-${os}-${arch}.tar.zst" -C "$outdir" .

    restore_placeholders "$static"
    log_info "Build complete: $workspace/revm-${os}-${arch}.tar.zst"
}

build "$@"
