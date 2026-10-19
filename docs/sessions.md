# Session commands

```sh
revm ls                     # ID, status and current/last instance
revm ls --json
revm inspect dev            # JSON config, volume reference and last run result
revm wait --ready --timeout 30s dev
revm wait dev               # return the recorded instance exit status
revm logs dev
revm logs --guest -f dev
revm stop --timeout 15s dev
revm rm dev                 # stopped sessions only; volumes remain
revm run -- sh -c 'echo temporary'
```

`wait` and `stop` bind to the instance selected when the command starts. They do
not silently follow a replacement instance using the same session name. `stop`
requests cancellation and waits for the instance to release its locks. The VM's
shutdown grace period is its startup `--stop-timeout`; `stop --timeout` bounds the
client's wait. An already stopped session is a successful no-op.

`wait --ready` waits until the guest execution service is ready after mounts and
network setup. `wait` without it returns the persisted CLI status. A crash with
no recorded result is `unknown` and returns an error. Inspect includes the guest
result separately from the CLI exit code, cleanup state and error text.

`logs` reads the latest instance's `revm.log`; `--guest` selects `vm.log`.
`--instance UUID` reads an older run. `--follow` ends when that instance stops.
Logs, configuration and results are private state; workload stdout/stderr are
streamed to the caller and are not automatically copied into these logs.

`rm` refuses a running session and removes its records, logs and owned stale
runtime/cache directories. Persistent volumes, external disks and bind mounts
remain. Runs without `--id` perform the same removal on success or failure. `--id NAME` retains records.

`ctl --id dev --list-port --json` returns the current port mapping array.

New sessions are published only after their initial records are complete.
Incomplete or unreadable records appear as `broken` with an `error` in JSON;
they do not block listing other sessions. `rm` can remove these records while
preserving volumes and external disks. Session names beginning with `.` are
reserved for internal staging.
