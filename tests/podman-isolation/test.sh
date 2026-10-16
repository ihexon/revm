#!/usr/bin/env bash
set -euo pipefail

# Run against a disposable, already-running revm session.
: "${CONTAINER_HOST:?Set CONTAINER_HOST to the revm Podman socket}"
PODMAN="${PODMAN:-podman}"
root="$(cd "$(dirname "$0")/../.." && pwd)"
work="$(mktemp -d)"
name="revm-isolation-test-$$"
trap '"$PODMAN" rm -f "$name" >/dev/null 2>&1 || true; "$PODMAN" rmi "$name" >/dev/null 2>&1 || true; rm -rf "$work"' EXIT
arch="$("$PODMAN" info --format '{{.Host.Arch}}')"
cd "$root"
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -o "$work/probe" ./tests/podman-isolation
cp tests/podman-isolation/Containerfile "$work/"
"$PODMAN" build -t "$name" "$work"
for network in bridge host; do
    for user in 0 10001; do
        "$PODMAN" run --rm --http-proxy=false --network="$network" --user="$user" \
            --security-opt seccomp=unconfined "$name"
    done
done

"$PODMAN" run --rm --http-proxy=false -v /run/revm/podman:/run/revm/podman:ro \
    "$name" /usr/local/bin/probe trusted

# HTTP upgrades, stdin half-close, stdout/stderr and archive transfer must all
# survive the transport without interpreting the Podman protocol.
"$PODMAN" run -d --name "$name" --http-proxy=false "$name" sh -c 'echo started; sleep 300'
[[ "$("$PODMAN" logs "$name")" == started ]]
printf 'stdin-through-upgrade\n' | "$PODMAN" exec -i "$name" cat > "$work/exec.out"
grep -Fx stdin-through-upgrade "$work/exec.out"
"$PODMAN" exec "$name" sh -c 'echo stdout; echo stderr >&2' > "$work/stdout" 2> "$work/stderr"
grep -Fx stdout "$work/stdout"
grep -Fx stderr "$work/stderr"
dd if=/dev/urandom of="$work/payload" bs=1048576 count=4 2>/dev/null
"$PODMAN" cp "$work/payload" "$name:/tmp/payload"
"$PODMAN" cp "$name:/tmp/payload" "$work/returned"
cmp "$work/payload" "$work/returned"
"$PODMAN" stop -t 1 "$name"
"$PODMAN" rm "$name"
"$PODMAN" run -d --name "$name" --http-proxy=false -p 127.0.0.1::8080 "$name" /usr/local/bin/probe serve
published="$("$PODMAN" port "$name" 8080/tcp)"
[[ "$(curl --noproxy '*' --retry 5 --retry-all-errors --retry-delay 1 --max-time 5 -fsS "http://$published/")" == podman-port-ok ]]
echo 'Podman isolation and streaming checks passed'
