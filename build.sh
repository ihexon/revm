#!/usr/bin/env bash
set -euo pipefail

# ─── Constants ────────────────────────────────────────────────────────────────

readonly ASSETS_BASE="https://github.com/ihexon/revm-assets/releases/download/v2.0.5"
readonly HOMEBREW_PREFIX="${HOMEBREW_PREFIX:-/opt/homebrew}"

readonly RED="\033[31m" GREEN="\033[32m" RESET="\033[0m"

# ─── Globals (set once by init_env) ───────────────────────────────────────────

OS=""          # Darwin | Linux
ARCH=""        # arm64  | aarch64
WORKSPACE=""
OUTDIR=""
BINDIR=""
LIBDIR=""
HELPERDIR=""
DEPSDIR="/tmp/.deps"
STATICDIR=""
PKGCFGDIR=""   # Darwin only: pkg-config search path
DIRTY=0

# ─── Logging ──────────────────────────────────────────────────────────────────

log_info() { echo -e "${GREEN}[INFO]${RESET} $*"; }
log_err()  { echo -e "${RED}[ERROR]${RESET} $*" >&2; exit 1; }

# ─── Helpers ──────────────────────────────────────────────────────────────────

fetch_asset() {
    local name="$1" url="$2" dest="$3"
    log_info "Fetching $name..."
    mkdir -p "$dest" && wget -qO- "$url" | bsdtar --zstd -x -C "$dest"
}

# ─── Init ─────────────────────────────────────────────────────────────────────

parse_args() {
    for arg in "$@"; do
        case "$arg" in
            --dirty) DIRTY=1 ;;
            *)       log_err "Unknown argument: $arg" ;;
        esac
    done
}

init_env() {
    OS="$(uname -s)"
    ARCH="$(uname -m)"
    WORKSPACE="$(cd "$(dirname "$0")" && pwd)"
    cd "$WORKSPACE"
    OUTDIR="$WORKSPACE/out"
    BINDIR="$OUTDIR/bin"
    LIBDIR="$OUTDIR/lib"
    HELPERDIR="$OUTDIR/helper"
    STATICDIR="$WORKSPACE/pkg/static_resources"

    rm -rf "$OUTDIR"
    mkdir -p "$BINDIR" "$LIBDIR" "$HELPERDIR"

    if [[ "$OS" == "Darwin" ]]; then
        PKGCFGDIR="$(brew --prefix libarchive)/lib/pkgconfig:$(brew --prefix e2fsprogs)/lib/pkgconfig"
    fi
}

# ─── Build Steps ──────────────────────────────────────────────────────────────

build_guest_agent() {
    log_info "Building guest-agent..."
    (cd "$WORKSPACE/cmd/guest-agent" &&
        CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
            -ldflags="-s -w" \
            -o "$HELPERDIR/guest-agent" main.go)
}

build_clean() {
    log_info "Building clean helper..."
    CGO_ENABLED=0 go build \
        -ldflags="-s -w" \
        -o "$HELPERDIR/clean" "$WORKSPACE/cmd/clean"
}

fetch_deps() {
    local asset_os asset_arch
    if [[ "$OS" == "Darwin" ]]; then
        asset_os="Darwin"  asset_arch="arm64"
    else
        asset_os="Linux"   asset_arch="aarch64"
    fi

    if [[ "$DIRTY" == 1 && -d "$DEPSDIR" ]]; then
        log_info "Dirty mode: reusing cached deps in $DEPSDIR"
    else
        [[ "$DIRTY" == 1 ]] && log_info "Dirty mode: $DEPSDIR not found, downloading anyway..."
        rm -rf "$DEPSDIR"
        mkdir -p "$DEPSDIR"
        fetch_asset "libkrun"   "$ASSETS_BASE/libkrun-${asset_os}-${asset_arch}.tar.zst"   "$DEPSDIR/libkrun"
        fetch_asset "libkrunfw" "$ASSETS_BASE/libkrunfw-${asset_os}-${asset_arch}.tar.zst" "$DEPSDIR/libkrunfw"
    fi

    # lib64 → lib symlinks so CGo header search works on Linux
    if [[ "$OS" == "Linux" ]]; then
        [[ -d "$DEPSDIR/libkrun/lib64"   && ! -e "$DEPSDIR/libkrun/lib" ]]   && ln -s lib64 "$DEPSDIR/libkrun/lib"
        [[ -d "$DEPSDIR/libkrunfw/lib64" && ! -e "$DEPSDIR/libkrunfw/lib" ]] && ln -s lib64 "$DEPSDIR/libkrunfw/lib"
    fi

    log_info "Fetching rootfs..."
    wget -qO "$STATICDIR/rootfs/rootfs.tar.zst" "$ASSETS_BASE/alpine-rootfs-Linux-aarch64.tar.zst"
}

relocate_libs_darwin() {
    cp -av "$DEPSDIR/libkrun/lib/"*.dylib "$DEPSDIR/libkrunfw/lib/"*.dylib "$LIBDIR/"
    cp -av "$HOMEBREW_PREFIX/opt/libepoxy/lib/libepoxy.0.dylib"             "$LIBDIR/"
    cp -av "$HOMEBREW_PREFIX/opt/virglrenderer/lib/libvirglrenderer.1.dylib" "$LIBDIR/"
    cp -av "$HOMEBREW_PREFIX/opt/molten-vk/lib/libMoltenVK.dylib"           "$LIBDIR/"
    rm -rf "$LIBDIR/pkgconfig"

    # Rewrite Homebrew absolute paths in libkrun.1.dylib
    install_name_tool -change "$HOMEBREW_PREFIX/opt/libepoxy/lib/libepoxy.0.dylib"             "@loader_path/libepoxy.0.dylib"             "$LIBDIR/libkrun.1.dylib" 2>/dev/null || true
    install_name_tool -change "$HOMEBREW_PREFIX/opt/virglrenderer/lib/libvirglrenderer.1.dylib" "@loader_path/libvirglrenderer.1.dylib"     "$LIBDIR/libkrun.1.dylib" 2>/dev/null || true
    install_name_tool -change "$HOMEBREW_PREFIX/opt/molten-vk/lib/libMoltenVK.dylib"           "@loader_path/libMoltenVK.dylib"             "$LIBDIR/libkrun.1.dylib" 2>/dev/null || true

    # Rewrite Homebrew absolute paths in libkrunfw.5.dylib
    install_name_tool -change "$HOMEBREW_PREFIX/opt/libepoxy/lib/libepoxy.0.dylib"             "@loader_path/libepoxy.0.dylib"             "$LIBDIR/libkrunfw.5.dylib" 2>/dev/null || true
    install_name_tool -change "$HOMEBREW_PREFIX/opt/virglrenderer/lib/libvirglrenderer.1.dylib" "@loader_path/libvirglrenderer.1.dylib"     "$LIBDIR/libkrunfw.5.dylib" 2>/dev/null || true
    install_name_tool -change "$HOMEBREW_PREFIX/opt/molten-vk/lib/libMoltenVK.dylib"           "@loader_path/libMoltenVK.dylib"             "$LIBDIR/libkrunfw.5.dylib" 2>/dev/null || true

    # Rewrite Homebrew absolute paths in libvirglrenderer.1.dylib
    install_name_tool -change "$HOMEBREW_PREFIX/opt/libepoxy/lib/libepoxy.0.dylib"             "@loader_path/libepoxy.0.dylib"             "$LIBDIR/libvirglrenderer.1.dylib" 2>/dev/null || true
    install_name_tool -change "$HOMEBREW_PREFIX/opt/molten-vk/lib/libMoltenVK.dylib"           "@loader_path/libMoltenVK.dylib"             "$LIBDIR/libvirglrenderer.1.dylib" 2>/dev/null || true

    # Fix install names and re-sign Homebrew dylibs
    install_name_tool -id "@loader_path/libepoxy.0.dylib"         "$LIBDIR/libepoxy.0.dylib"
    codesign --force -s - "$LIBDIR/libepoxy.0.dylib"
    install_name_tool -id "@loader_path/libvirglrenderer.1.dylib" "$LIBDIR/libvirglrenderer.1.dylib"
    codesign --force -s - "$LIBDIR/libvirglrenderer.1.dylib"
    install_name_tool -id "@loader_path/libMoltenVK.dylib"        "$LIBDIR/libMoltenVK.dylib"
    codesign --force -s - "$LIBDIR/libMoltenVK.dylib"

    # Fix revm binary's dylib references (revm is in bin/, dylibs are in lib/)
    install_name_tool -change "libkrun.1.dylib"  "@loader_path/../lib/libkrun.1.dylib"  "$BINDIR/revm"
    install_name_tool -change "libkrunfw.5.dylib" "@loader_path/../lib/libkrunfw.5.dylib" "$BINDIR/revm"
    codesign --entitlements "$WORKSPACE/revm.entitlements" --force -s - "$BINDIR/revm"

    PKG_CONFIG_PATH="$PKGCFGDIR" golangci-lint run
}

relocate_libs_linux() {
    cp -av "$DEPSDIR/libkrun/lib64/"*.so*   "$LIBDIR/"
    cp -av "$DEPSDIR/libkrunfw/lib64/"*.so* "$LIBDIR/"

    # Collect all .so deps (skip ld-linux, it goes to HELPERDIR)
    LD_LIBRARY_PATH="$LIBDIR" ldd "$BINDIR/revm" | grep -o "/.* " | while read -r lib; do
        [[ "$(basename "$lib")" == ld-linux* ]] && continue
        local dst="$LIBDIR/$(basename "$lib")"
        [[ -e "$dst" ]] && continue
        cp -Lv "$lib" "$LIBDIR/"
    done

    # Copy the dynamic linker for ld-linux wrapper use
    cp -Lv /lib/ld-linux-aarch64.so.1 "$HELPERDIR/"

    patchelf --set-rpath '$ORIGIN/../lib' "$BINDIR/revm"
    for sofile in "$LIBDIR"/libkrun*.so.*.*; do
        patchelf --set-rpath '$ORIGIN' "$sofile"
    done

    # Rename CGo binary to hidden file; the Go wrapper becomes the user-facing "revm"
    mv "$BINDIR/revm" "$BINDIR/.revm"

    # Build Go wrapper as the user-facing "revm" (exec's .revm via ld-linux)
    CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BINDIR/revm" "$WORKSPACE/cmd/revm-helper"
}

relocate_libs() {
    log_info "Preparing shared libraries..."
    if [[ "$OS" == "Darwin" ]]; then
        relocate_libs_darwin
    else
        relocate_libs_linux
    fi
}

build_revm() {
    local version commit
    version="$(git -C "$WORKSPACE" describe --tags --abbrev=0 2>/dev/null || echo "unknown")"
    commit="$(git  -C "$WORKSPACE" rev-parse --short HEAD     2>/dev/null || echo "unknown")"
    log_info "Building revm ($version-$commit)..."

    local ldflags="-X linuxvm/pkg/define.Version=$version -X linuxvm/pkg/define.CommitID=$commit"

    if [[ "$OS" == "Darwin" ]]; then
        PKG_CONFIG_PATH="$PKGCFGDIR" \
            go build -ldflags="$ldflags" -o "$BINDIR/revm" "$WORKSPACE/cmd/revm"
    else
        CGO_ENABLED=1 \
            go build -ldflags="$ldflags" -o "$BINDIR/revm" "$WORKSPACE/cmd/revm"
    fi
}

package() {
    log_info "Packaging..."
    bsdtar --zstd -cf "$WORKSPACE/revm-${OS}-${ARCH}.tar.zst" -C "$OUTDIR" .

    # Restore placeholder so the working tree stays clean.
    : >"$STATICDIR/rootfs/rootfs.tar.zst"

    log_info "Build complete: $WORKSPACE/revm-${OS}-${ARCH}.tar.zst"
}

# ─── Main ─────────────────────────────────────────────────────────────────────

main() {
    parse_args "$@"
    init_env

    build_guest_agent
    build_clean
    fetch_deps
    build_revm
    relocate_libs
    package
}

main "$@"
