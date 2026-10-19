#!/bin/sh
# Run after the build has populated /tmp/.deps.
set -eu
cd "$(dirname "$0")/.."
if [ "$(uname -s)" = Darwin ]; then
  PKG_CONFIG_PATH="$(brew --prefix e2fsprogs)/lib/pkgconfig:$(brew --prefix libarchive)/lib/pkgconfig${PKG_CONFIG_PATH:+:$PKG_CONFIG_PATH}"
  DYLD_LIBRARY_PATH="/tmp/.deps/libkrun/lib:/tmp/.deps/libkrunfw/lib${DYLD_LIBRARY_PATH:+:$DYLD_LIBRARY_PATH}"
  export PKG_CONFIG_PATH DYLD_LIBRARY_PATH
else
  LD_LIBRARY_PATH="/tmp/.deps/libkrun/lib:/tmp/.deps/libkrunfw/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
  export LD_LIBRARY_PATH
fi
go test -race ./...
if [ "$(uname -s)" = Linux ]; then
  (cd cmd/guest-agent && go test ./...)
else
  # The guest is Linux-only. Compile its test binary on macOS; CI runs it on Linux.
  test_bin=$(mktemp /tmp/revm-guest-tests.XXXXXX)
  trap 'rm -f "$test_bin"' EXIT HUP INT TERM
  (cd cmd/guest-agent && CGO_ENABLED=0 GOOS=linux go test -c -o "$test_bin" ./pkg/vsock)
fi
