#!/bin/bash
set -euo pipefail

readonly ASSETS_BASE="https://github.com/ihexon/revm-assets/releases/download/v2.0.4"
readonly GO_VERSION="1.25.5"
readonly GO_URL="https://go.dev/dl/go${GO_VERSION}.linux-arm64.tar.gz"

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
	mkdir -p "$dest" && wget -qO- "$url" | tar --zstd -x -C "$dest"
}

# Restore placeholder files for git cleanliness
restore_placeholders() {
	local static="$1"
	local files=(
		"$static/guest-agent/guest-agent"
		"$static/rootfs/rootfs.tar.zst"
	)

	for f in "${files[@]}"; do
		: > "$f"
	done
}

build() {
	local workspace="$(cd "$(dirname "$0")" && pwd)"
	local outdir="$workspace/out"
	local bindir="$outdir/bin"
	local depsdir="$outdir/.deps"
	local static="$workspace/pkg/static_resources"
	local go_root="$outdir/.go/go"

	rm -rf "$outdir"
	mkdir -p "$bindir" "$depsdir"

	log_info "Using Go: $(go version)"

	# 2. Build guest-agent (must be static for Alpine musl rootfs)
	log_info "Building guest-agent..."
	(cd "$workspace/guest-agent" \
		&& CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
			-ldflags="-s -w" \
			-o "$static/guest-agent/guest-agent" main.go)

	# 3. Fetch dependencies
	fetch_asset "e2fsprogs" "$ASSETS_BASE/e2fsprogs-Linux-aarch64.tar.zst" "$depsdir/e2fsprogs"
	fetch_asset "libkrun" "$ASSETS_BASE/libkrun-Linux-aarch64.tar.zst" "$depsdir/libkrun"
	fetch_asset "libkrunfw" "$ASSETS_BASE/libkrunfw-Linux-aarch64.tar.zst" "$depsdir/libkrunfw"

	log_info "Fetching rootfs..."
	wget -qO "$static/rootfs/rootfs.tar.zst" "$ASSETS_BASE/alpine-rootfs-Linux-aarch64.tar.zst"

	# 4. Create compatibility symlinks (archives use lib64/ but CGO expects lib/)
	if [[ -d "$depsdir/libkrun/lib64" ]] && [[ ! -e "$depsdir/libkrun/lib" ]]; then
		ln -s lib64 "$depsdir/libkrun/lib"
	fi
	if [[ -d "$depsdir/libkrunfw/lib64" ]] && [[ ! -e "$depsdir/libkrunfw/lib" ]]; then
		ln -s lib64 "$depsdir/libkrunfw/lib"
	fi

	# 5. Copy shared libraries to bindir
	log_info "Preparing shared libraries..."
	cp -av "$depsdir/libkrun/lib64/"*.so* "$bindir/"
	cp -av "$depsdir/libkrunfw/lib64/"*.so* "$bindir/"

	# 6. Build revm
	local version commit
	version="$(git -C "$workspace" describe --tags --abbrev=0 2> /dev/null || echo "unknown")"
	commit="$(git -C "$workspace" rev-parse --short HEAD 2> /dev/null || echo "unknown")"
	log_info "Building revm ($version-$commit)..."

	export CGO_ENABLED=1

	# libarchive-go is missing cgo_linux_arm64.go, so we must supply -larchive here.
	# -luuid at the end resolves circular static-lib dependencies (libblkid.a -> libuuid.a).
	export CGO_LDFLAGS="-larchive -luuid"

	go build -ldflags="-X linuxvm/pkg/define.Version=$version -X linuxvm/pkg/define.CommitID=$commit" \
		-o "$bindir/revm" "$workspace/cmd/revm"

	patchelf --set-rpath '$ORIGIN' "$bindir/revm"
	for sofile in "$bindir"/libkrun*.so.*.*; do
		patchelf --set-rpath '$ORIGIN' "$sofile"
	done

	# 8. Package
	rm -rf "$depsdir"
	log_info "Packaging..."
	tar --zstd -cf "$workspace/revm-$(uname -s)-$(uname -m).tar.zst" -C "$outdir" .

	# 9. Restore placeholder files
	restore_placeholders "$static"

	log_info "Build complete: $workspace/revm-$(uname -s)-$(uname -m).tar.zst"
}

build "$@"
