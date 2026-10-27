# Execution protocol v3

Run and exec start commands through the same guest-control service. The service
becomes ready after filesystem mounts and network configuration. Older guests
without v3 return a capability error and must be restarted.

An HTTP upgrade carries four Yamux streams: control, stdin, stdout and stderr.
Each stream has a bounded 256 KiB flow-control window; control traffic is not
queued behind command data. Stdin streams immediately and its write-side EOF
does not close output. PTY mode combines output on stdout and does not translate
input EOF into Ctrl-D. Commands are argv arrays, including when using a PTY.

Execution requests carry the guest instance identifier to reject stale targets.
One connection starts one execution; there are no request or execution IDs.
The host never retries a start after an ambiguous transport failure. After the
`started` notification, the guest sends one `completed` result after command
cleanup and output forwarding. The client acknowledges it after draining both
output streams. Before that result, a disconnect means the status is unknown.
The only ongoing control message is PTY resize.

Each guest command starts atomically in its own cgroup v2 subtree. Cancellation
and completion kill the whole subtree, including descendants using setsid. The
command sees only a read-only cgroup mount and cannot regain mount/ptrace
privileges. Commands therefore do not have unrestricted CAP_SYS_ADMIN inside
the VM; the Podman service itself is managed separately by the guest agent.
Executions in a Podman session default `CONTAINER_HOST` to that service's Unix
socket. Guest readiness in this mode also waits for the engine to answer ping,
so `exec -- podman ps` remains usable without local mount privileges.

`--tty=auto` requires both stdin and stdout to be terminals, with revm in the
foreground. `always` and `never` override allocation.
The host enters raw mode after `started`, retaining terminal-generated signals.
Ctrl-C cancels locally, and suspend/exit restore the terminal. Resize reaches the
guest PTY. Background jobs must use `--tty=never`; redirected stdin is allowed.

The revm process directly hosts libkrun and host services. There is no `_vmm`
child, shutdown RPC, escalation timer or native teardown while the VM runs.
Cancellation returns immediately and main ends the entire process. Disconnecting
an exec client kills its guest command subtree without stopping the VM. Normal
command completion drains output before process exit. There is no automatic
sync: commands and applications own flushing their writes.

If libkrun calls `_exit` itself, the shell receives its native exit status.
There is no interception layer. Without a recorded command result the session
stays `unknown`, and `revm wait` returns 1, even if the native status was 0.

A detached `_cleanup` helper inherits instance locks, the original terminal
state and a read-only lifetime pipe. Only the VM process keeps the write end.
EOF means all VM threads have died, so the helper can safely remove temporary
resources and release locks. It never inherits stdout/stderr pipes or consumes
stdin. A missing result stays unknown. The CLI may finish before file cleanup;
`wait` waits for the locks too.
Terminal restoration after an abrupt exit is best effort; an already revoked
or closed controlling terminal cannot be restored.
Persistent data is never removed. `cache prune` recovers leftovers if cleanup
itself is interrupted.

The guest agent owns required services and every upgraded execution. Service
failure ends the VM; services are not restarted. `/v1/exec` and `/v1/shutdown`
are removed. revm never requests guest shutdown or filesystem sync; applications requiring
persistence must flush writes before exiting or stopping the VM.
