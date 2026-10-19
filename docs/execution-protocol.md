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

The current execution supervisor kills the command process group on disconnect
and completion. The signal/terminal implementation adds the guest cgroup boundary
needed to cover descendants that leave the group with setsid.
