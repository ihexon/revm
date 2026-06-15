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

verify_libkrun_init_blob() {
    local init_bin init_file
    init_bin="$(find "$LIBKRUN_SRC/target/release/build" -path "*/out/init" -type f -print -quit)"
    if [[ -z "$init_bin" ]]; then
        echo "libkrun init blob not found" >&2
        exit 100
    fi

    init_file="$(file -b "$init_bin")"
    echo "libkrun init blob: $init_file"
    if [[ "$init_file" != *ELF* || "$init_file" != *static* ]]; then
        echo "libkrun init blob must be a static Linux ELF" >&2
        exit 100
    fi
}
