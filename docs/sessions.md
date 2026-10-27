# Session commands

```sh
revm ls                     # mode, status, start time, duration, exit code and command
revm ls --json
revm inspect dev            # JSON config, last result, log directory and live endpoints
revm wait --ready --timeout 30s dev
revm wait dev               # return the recorded instance exit status
revm logs --tail 50 dev
revm logs --guest --tail 20 -f dev
revm stop --timeout 15s dev
revm rm dev                 # stopped sessions only; volumes remain
revm run -- sh -c 'echo temporary'
```

`wait` and `stop` bind to the instance selected when the command starts. They do
not silently follow a replacement instance using the same session name. `stop`
ends the VM process immediately and waits for its cleanup helper to release the
instance locks. `stop --timeout` bounds the client's wait. An already stopped session is a successful no-op.

`wait --ready` waits until the guest execution service is ready after mounts and
network setup. `wait` without it waits for post-exit cleanup and returns the recorded status.
Cleanup failures may change that recorded status after the originating shell has
already received the command exit code. A crash with
no recorded result is `unknown` and returns an error. Inspect includes the guest
result separately from the CLI exit code, cleanup state and error text.

`logs` reads the latest instance's `revm.log`; `--guest` selects `vm.log`.
`--tail N` reads the last N lines before following, without loading the entire
file. The default `--tail -1` reads all lines; `--tail 0 -f` follows new lines only.
`--instance UUID` reads an older run. `--follow` ends when that instance stops.
Logs, configuration and results are private state; workload stdout/stderr are
streamed to the caller and are not automatically copied into these logs.

`rm` refuses a running session and removes its records, logs and owned stale
runtime/cache directories. Persistent volumes, external disks and bind mounts
remain. Runs without `--id` remove records only after successful execution and
cleanup. Failures, signal interruptions and crashes retain their records under
an automatically generated name, visible in `revm ls`. Use `inspect`, `logs` and
`rm` with that name. `cache prune` preserves diagnostic records. `--id NAME`
always retains records.

`revm ports --json dev` returns the current port mapping array.

New sessions are published only after their initial records are complete.
Incomplete or unreadable records appear as `broken` with an `error` in JSON;
they do not block listing other sessions. `rm` can remove these records while
preserving volumes and external disks. Session names beginning with `.` are
reserved for internal staging.

`inspect` and `ls --json` include `logDir`. While the selected instance is active,
`endpoints.management` and (for Podman mode) `endpoints.podman` are Unix URLs.
These are connection addresses, not readiness guarantees; use `wait --ready`
for guest readiness and the Podman API ping for Podman readiness. Stopped
instances omit `endpoints` so scripts do not reuse stale sockets.

```sh
export CONTAINER_HOST="$(revm inspect containers | jq -r '.endpoints.podman')"
podman info
```

Human-readable `ls` shows UTC start timestamps and elapsed instance time.
Unknown crash duration and missing exit codes display `-`; `inspect` retains
the full error and instance ID. Use JSON for scripts, not table column parsing.
