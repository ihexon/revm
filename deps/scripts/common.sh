#!/usr/bin/env bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPS_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
REPO_ROOT="$(cd "$DEPS_DIR/.." && pwd)"

# shellcheck source=../sources.lock
source "$DEPS_DIR/sources.lock"

PLT="$(uname)"
ARCH="$(uname -m)"

DEPS_WORK_DIR="${DEPS_WORK_DIR:-$DEPS_DIR/.work}"
DEPS_DIST_DIR="${DEPS_DIST_DIR:-$DEPS_DIR/dist}"

mkdir -p "$DEPS_WORK_DIR" "$DEPS_DIST_DIR"

rust_linux_musl_target() {
    case "$(uname -m)" in
        arm64|aarch64)
            echo "aarch64-unknown-linux-musl"
            ;;
        x86_64|amd64)
            echo "x86_64-unknown-linux-musl"
            ;;
        *)
            echo "unsupported architecture for Linux musl target: $(uname -m)" >&2
            return 1
            ;;
    esac
}

install_rust_linux_musl_target() {
    local target
    target="$(rust_linux_musl_target)"

    if [[ -n "${RUSTUP_TOOLCHAIN:-}" ]]; then
        rustup target add --toolchain "$RUSTUP_TOOLCHAIN" "$target"
    else
        rustup target add "$target"
    fi

    if [[ "$(uname)" == "Darwin" ]]; then
        case "$target" in
            aarch64-unknown-linux-musl)
                export CARGO_TARGET_AARCH64_UNKNOWN_LINUX_MUSL_LINKER="${CARGO_TARGET_AARCH64_UNKNOWN_LINUX_MUSL_LINKER:-rust-lld}"
                ;;
            x86_64-unknown-linux-musl)
                export CARGO_TARGET_X86_64_UNKNOWN_LINUX_MUSL_LINKER="${CARGO_TARGET_X86_64_UNKNOWN_LINUX_MUSL_LINKER:-rust-lld}"
                ;;
        esac
    fi
}

# libkrun's current ffier generator aborts with BrokenPipe while formatting the
# 2.0 schema. Formatting is not part of the generated ABI, so patch only this
# build-time helper after Cargo has fetched it and let rustc validate the source.
patch_ffier_generator() {
    local libkrun_src="$1"
    local cargo_home="${CARGO_HOME:-$HOME/.cargo}"
    cargo fetch --locked --manifest-path "$libkrun_src/Cargo.toml"
    while IFS= read -r generator; do
        python3 - "$generator" <<'PY'
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
source = path.read_text()
start_marker = "fn rustfmt(src: &str) -> String {"
end_marker = "\n}\n\n/// Generate from a JSON file path."
if start_marker not in source or "src.to_owned()" in source:
    raise SystemExit(0)
start = source.index(start_marker)
end = source.index(end_marker, start) + 2
replacement = "fn rustfmt(src: &str) -> String {\n    src.to_owned()\n}"
path.write_text(source[:start] + replacement + source[end:])
PY
    done < <(find "$cargo_home/git/checkouts" -path '*/ffier-gen-rust-client/src/lib.rs' -type f -print)
}

verify_libkrun_bundle() {
    local lib_dir="$1"
    local libkrun_file="$lib_dir/libkrun.so.2.0.0"
    local init_file="$lib_dir/libkrun_init.so.0.1.0"
    if [[ "$(uname)" == "Darwin" ]]; then
        libkrun_file="$lib_dir/libkrun.2.0.0.dylib"
        init_file="$lib_dir/libkrun_init.0.1.0.dylib"
    fi
    if [[ ! -f "$libkrun_file" || ! -f "$init_file" ]]; then
        echo "libkrun 2.x shared libraries not found in $lib_dir" >&2
        find "$lib_dir" -maxdepth 1 -type f -print >&2 || true
        exit 100
    fi
}
