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

# libkrun's generated ffier clients invoke `rustfmt` from a Cargo build script.
# Some upstream schemas currently make rustfmt abort before consuming stdin,
# which surfaces as a misleading BrokenPipe panic in ffier-gen-rust-client.
# Buffer the input and fall back to the valid generated source when formatting
# fails; rustc remains the authoritative syntax/type checker for the client.
prepare_ffier_rustfmt_compat() {
    local real_rustfmt wrapper_dir wrapper
    real_rustfmt="$(rustup which rustfmt 2>/dev/null || command -v rustfmt)"
    wrapper_dir="$DEPS_WORK_DIR/bin"
    wrapper="$wrapper_dir/rustfmt"
    mkdir -p "$wrapper_dir"
    cat > "$wrapper" <<EOF
#!/usr/bin/env bash
set -euo pipefail
input=\"\$(mktemp)\"
trap 'rm -f "\$input"' EXIT
cat > "\$input"
if "$real_rustfmt" "\$@" < "\$input"; then
    exit 0
fi
echo "warning: upstream ffier rustfmt failed; using unformatted generated client" >&2
cat "\$input"
EOF
    chmod +x "$wrapper"
    export PATH="$wrapper_dir:$PATH"
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
