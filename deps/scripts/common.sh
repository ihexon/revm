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
