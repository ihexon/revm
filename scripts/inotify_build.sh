#!/usr/bin/env bash
set -euo pipefail

DIR="${1:-.}"
BUILD_SCRIPT="$DIR/build.sh"
DEBOUNCE_SECONDS=1

if [[ ! -x "$BUILD_SCRIPT" ]]; then
    echo "Error: $BUILD_SCRIPT not found or not executable"
    exit 1
fi

echo "Watching directory: $DIR"

running=0
last_run=0

trigger_build() {
    if [[ "$running" -eq 1 ]]; then
        echo "Build already running, skipping..."
        return
    fi

    now=$(date +%s)
    if ((now - last_run < DEBOUNCE_SECONDS)); then
        return
    fi

    running=1
    last_run=$now

    echo "=== Build triggered at $(date) ==="

    (
        cd "$DIR"
        ./build.sh
    )

    echo "=== Build finished at $(date) ==="
    running=0
}

inotifywait -m -r \
    -e modify -e create -e delete -e move \
    --exclude '(^|/)\.git/' \
    "$DIR" |
    while read -r path event file; do
        echo "Change detected: $event $path$file"
        trigger_build
    done
