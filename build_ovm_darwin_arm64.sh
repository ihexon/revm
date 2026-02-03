#!/opt/homebrew/bin/bash
set -euo pipefail

readonly ASSETS_BASE="https://github.com/ihexon/revm-assets/releases/download/v2.0.0"
readonly HOMEBREW_PREFIX="${HOMEBREW_PREFIX:-/opt/homebrew}"

readonly E2FS_BINS=(blkid tune2fs mke2fs e2fsck)
readonly HOMEBREW_DYLIBS=(
    libepoxy/lib/libepoxy.0.dylib
    virglrenderer/lib/libvirglrenderer.1.dylib
    molten-vk/lib/libMoltenVK.dylib
)

# Colors
readonly RED="\033[31m" GREEN="\033[32m" RESET="\033[0m"

log_info() { echo -e "${GREEN}[INFO]${RESET} $*"; }
log_err() {
    echo -e "${RED}[ERROR]${RESET} $*" >&2
    exit 1
}

# Download and extract a tarball
fetch_asset() {
    local name="$1" url="$2" dest="$3"
    log_info "Fetching $name..."
    mkdir -p "$dest" && wget -qO- "$url" | tar -xz -C "$dest"
}

# Fix dylib install name and re-sign
fix_dylib() {
    local lib="$1" new_id="@loader_path/$(basename "$lib")"
    install_name_tool -id "$new_id" "$lib"
    codesign --force -s - "$lib"
}

# Rewrite dependency path in a binary
rewrite_dep() {
    local binary="$1" old_path="$2" new_name="$3"
    install_name_tool -change "$old_path" "@loader_path/$new_name" "$binary" 2>/dev/null || true
}

# Restore placeholder files for git cleanliness
restore_placeholders() {
    local static="$1"
    local files=(
        "$static/guest-agent/guest-agent"
        "$static/rootfs/rootfs.tar.zst"
    )
    for bin in "${E2FS_BINS[@]}"; do
        files+=("$static/e2fsprogs/$bin")
    done
    for f in "${files[@]}"; do
        : >"$f"
    done
}

build() {
    local workspace="$(cd "$(dirname "$0")" && pwd)"
    local outdir="$workspace/out"
    local bindir="$outdir/bin"
    local depsdir="$outdir/.deps"
    local static="$workspace/pkg/static_resources"

    rm -rf "$outdir"
    mkdir -p "$bindir" "$depsdir"

    # 1. Build guest-agent
    log_info "Building guest-agent..."
    (cd "$workspace/guest-agent" && ./build.sh linux arm64 "$static/guest-agent/guest-agent")

    # 2. Fetch dependencies
    fetch_asset "e2fsprogs" "$ASSETS_BASE/e2fsprogs-Darwin-arm64.tar.zst" "$depsdir/e2fsprogs"
    fetch_asset "libkrun" "$ASSETS_BASE/libkrun-Darwin-arm64.tar.zst" "$depsdir/libkrun"
    fetch_asset "libkrunfw" "$ASSETS_BASE/libkrunfw-Darwin-arm64.tar.zst" "$depsdir/libkrunfw"

    log_info "Fetching rootfs..."
    wget -qO "$static/rootfs/rootfs.tar.zst" "$ASSETS_BASE/alpine-rootfs-Linux-aarch64.tar.zst"

    # Copy e2fsprogs binaries
    for bin in "${E2FS_BINS[@]}"; do
        cp -av "$depsdir/e2fsprogs/sbin/$bin" "$static/e2fsprogs/$bin"
    done

    # 3. Prepare dylibs (must be done before go build for proper linking)
    log_info "Preparing dylibs..."
    cp -av "$depsdir/libkrun/lib/"*.dylib "$depsdir/libkrunfw/lib/"*.dylib "$bindir/"
    for dylib in "${HOMEBREW_DYLIBS[@]}"; do
        cp -av "$HOMEBREW_PREFIX/opt/$dylib" "$bindir/"
    done
    rm -rf "$bindir/pkgconfig"

    # Fix dylib paths and re-sign (install_name_tool invalidates signatures)
    for lib in "$bindir"/libkrun*.dylib; do
        for dylib in "${HOMEBREW_DYLIBS[@]}"; do
            local name
            name="$(basename "$dylib")"
            rewrite_dep "$lib" "$HOMEBREW_PREFIX/opt/$dylib" "$name"
        done
    done

    # Fix virglrenderer dependencies (depends on libepoxy and libMoltenVK)
    rewrite_dep "$bindir/libvirglrenderer.1.dylib" "$HOMEBREW_PREFIX/opt/libepoxy/lib/libepoxy.0.dylib" "libepoxy.0.dylib"
    rewrite_dep "$bindir/libvirglrenderer.1.dylib" "$HOMEBREW_PREFIX/opt/molten-vk/lib/libMoltenVK.dylib" "libMoltenVK.dylib"

    for dylib in "${HOMEBREW_DYLIBS[@]}"; do
        fix_dylib "$bindir/$(basename "$dylib")"
    done

    # golang-ci lint
    golangci-lint run

    # 4. Build revm (after dylibs are ready)
    local version commit
    version="$(git -C "$workspace" describe --tags --abbrev=0 2>/dev/null || echo "unknown")"
    commit="$(git -C "$workspace" rev-parse --short HEAD 2>/dev/null || echo "unknown")"
    log_info "Building revm ($version-$commit)..."

    go build -ldflags="-X linuxvm/pkg/define.Version=$version -X linuxvm/pkg/define.CommitID=$commit" \
        -o "$bindir/revm" "$workspace/cmd"

    # 5. Fix revm dependencies and sign
    rewrite_dep "$bindir/revm" "libkrun.1.dylib" "libkrun.1.dylib"
    rewrite_dep "$bindir/revm" "libkrunfw.5.dylib" "libkrunfw.5.dylib"
    codesign --entitlements "$workspace/revm.entitlements" --force -s - "$bindir/revm"

    # 6. Package
    rm -rf "$depsdir"
    log_info "Packaging..."
    tar --zstd -cf "$workspace/revm-$(uname -s)-$(uname -m).tar.zst" -C "$outdir" .

    # 7. Restore placeholder files
    restore_placeholders "$static"

    log_info "Build complete: $workspace/revm-$(uname -s)-$(uname -m).tar.zst"
}

build "$@"
