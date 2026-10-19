# Execution protocol v2

Run and attach start commands through the same guest-control service. The service
becomes ready after filesystem mounts and network configuration. Older guests
without v2 return a capability error and must be restarted.

An HTTP upgrade carries four Yamux streams: control, stdin, stdout and stderr.
Each stream has a bounded 256 KiB flow-control window; control traffic is not
queued behind command data. Stdin streams immediately and its write-side EOF
does not close output. PTY mode combines output on stdout and does not translate
input EOF into Ctrl-D. Commands are argv arrays, including when using a PTY.

Execution requests carry an instance identifier and a unique request identifier.
The guest rejects duplicate starts for that instance. The host never retries a
start after an ambiguous transport failure. Results distinguish process exit
from completed cleanup and output delivery; the client acknowledges completion
after draining both output streams.

Each guest command starts atomically in its own cgroup v2 subtree. Cancellation
and completion kill the whole subtree, including descendants using setsid. The
command sees only a read-only cgroup mount and cannot regain mount/ptrace
privileges. Commands therefore do not have unrestricted CAP_SYS_ADMIN inside
the VM; the Podman service itself is managed separately by the guest agent.
Executions in a Podman session default `CONTAINER_HOST` to that service's Unix
socket. Guest readiness in this mode also waits for the engine to answer ping,
so `attach -- podman ps` remains usable without local mount privileges.

`--tty=auto` requires both stdin and stdout to be terminals, with revm in the
foreground. `always` and `never` override allocation; `--pty` aliases `always`.
The host enters raw mode only after command startup, restores it on suspend/exit,
and forwards terminal resize. Stdin EOF never synthesizes terminal input.

INT, TERM, HUP and QUIT retain the first cancellation cause and deadline.
`--stop-timeout` defaults to 10s; a second signal accelerates cleanup. Attach
cancels only its execution. Run shares one grace budget between command cleanup
and VM shutdown, followed by at most 3s for forced shutdown. Normal output drain has no deadline: slow consumers apply backpressure. Only
cancellation bounds draining to 3s; incomplete output and stdin read failures
are errors. An early close by a command that no longer needs stdin is allowed.

The native VMM lives in a child process on a separate session. An inherited
control pipe ties its lifetime to its CLI owner, including during native
initialization. EOF exits the child even when a native thread is blocked; normal
shutdown waits for it. Killing the CLI can leave a briefly reparented child, but
must not leave a running orphan VMM. `tests/smoke.py` checks both startup and
running owner death on a real VM.

The guest agent owns required services and every upgraded execution. A service
failure ends the VM; services are not automatically restarted. Shutdown stops
accepting commands, sends TERM to executions, stops Podman containers, and waits
for service cleanup before syncing disks. These steps share a five-second guest
budget; the host timeout remains the final limit. The old `/v1/exec` endpoint is
removed.
