#!/usr/bin/env bash
set -euo pipefail

# ── Helpers ──────────────────────────────────────────────────────────────────

die()      { echo "[ERROR] $*" >&2; exit 1; }
log()      { echo "[INFO]  $*"; }
require()  { command -v "$1" &>/dev/null || die "required tool not found: $1"; }

workspace() { cd "$(dirname "$0")/.." && pwd; }

OS="$(uname -s)"   # Darwin or Linux

# ── Steps ─────────────────────────────────────────────────────────────────────

parse_args() {
    [[ $# -lt 2 || "$1" != "--revm-install-dir" ]] && {
        echo "Usage: $0 --revm-install-dir <path>" >&2; exit 1
    }
    revm_dir="$2"
    [[ -f "$revm_dir/bin/revm" ]] || die "bin/revm not found in $revm_dir"
}

content_hash() {
    if [[ "$OS" == "Darwin" ]]; then
        shasum -a 256 "$1" | cut -c1-16
    else
        sha256sum "$1" | cut -c1-16
    fi
}

populate_payload() {
    log "Populating payload from $revm_dir ..."
    local payload_tar="$ws/single-binary/payload.tar"

    build_id="$(content_hash "$revm_dir/bin/revm")"
    log "build_id: $build_id"

    # Archive preserving symlinks and permissions
    tar cf "$payload_tar" -C "$revm_dir" .
    log "payload.tar: $(du -h "$payload_tar" | cut -f1)"
}

build_binary() {
    log "Building $single_bin ..."
    cd "$ws"
    CGO_ENABLED=0 go build -ldflags="-s -w -X main.buildID=$build_id" -o "$single_bin" ./single-binary
}

sign_binary() {
    if [[ "$OS" != "Darwin" ]]; then
        log "Skipping code signing (not macOS)"
        return
    fi
    log "Signing $single_bin ..."
    codesign --entitlements "$ws/revm.entitlements" --force -s - "$single_bin"
}

package_binary() {
    local tarball="$ws/revm-single-$(uname -s)-$(uname -m).tar.zst"
    log "Packaging $tarball ..."
    bsdtar --zstd -cf "$tarball" -C "$(dirname "$single_bin")" "$(basename "$single_bin")"
    rm -f "$single_bin"
    log "Package: $tarball"
}

cleanup_payload() {
    : > "$ws/single-binary/payload.tar"
}

# ── Main ──────────────────────────────────────────────────────────────────────

main() {
    parse_args "$@"

    require bsdtar
    if [[ "$OS" == "Darwin" ]]; then
        require shasum
        require codesign
    else
        require sha256sum
    fi

    ws="$(workspace)"
    single_bin="$ws/revm-single-$(uname -s)-$(uname -m)"

    populate_payload
    build_binary
    sign_binary
    package_binary
    cleanup_payload

    log "Done: $single_bin"
}

main "$@"
