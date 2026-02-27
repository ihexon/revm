#!/usr/bin/env bash
set -euo pipefail

# ── Helpers ──────────────────────────────────────────────────────────────────

die()      { echo "[ERROR] $*" >&2; exit 1; }
log()      { echo "[INFO]  $*"; }
require()  { command -v "$1" &>/dev/null || die "required tool not found: $1"; }

workspace() { cd "$(dirname "$0")/.." && pwd; }

# ── Steps ─────────────────────────────────────────────────────────────────────

parse_args() {
    [[ $# -lt 2 || "$1" != "--revm-install-dir" ]] && {
        echo "Usage: $0 --revm-install-dir <path>" >&2; exit 1
    }
    revm_dir="$2"
    [[ -f "$revm_dir/bin/revm" ]] || die "bin/revm not found in $revm_dir"
}

populate_payload() {
    log "Populating payload from $revm_dir ..."
    rm -rf "$payload_dir"
    cp -r "$revm_dir/." "$payload_dir/"
    shasum -a 256 "$payload_dir/bin/revm" | cut -c1-16 > "$payload_dir/build_id"
    log "build_id: $(cat "$payload_dir/build_id")"
}

build_binary() {
    log "Building $single_bin ..."
    CGO_ENABLED=0 go build -ldflags="-s -w" -o "$single_bin" "$ws/single-binary"
}

sign_binary() {
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

restore_placeholder() {
    rm -rf "$payload_dir"
    mkdir -p "$payload_dir" && touch "$payload_dir/empty"
}

# ── Main ──────────────────────────────────────────────────────────────────────

main() {
    parse_args "$@"

    require shasum
    require codesign
    require bsdtar

    ws="$(workspace)"
    payload_dir="$ws/single-binary/payload"
    single_bin="$ws/revm-single-$(uname -s)-$(uname -m)"

    populate_payload
    build_binary
    sign_binary
    package_binary
    restore_placeholder

    log "Done: $single_bin"
}

main "$@"
