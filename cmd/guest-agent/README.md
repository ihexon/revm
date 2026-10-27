# Guest agent

The internal Linux process at `/.bin/guest-agent` mounts the guest filesystems,
configures networking, and owns the command service and optional Podman engine.
The host injects this executable into the separately built Alpine rootfs.

Commands and PTYs use `/v3/exec` over a host-only vsock listener. Four Yamux
streams carry control, stdin, stdout and stderr. Each command gets a cgroup;
cancellation and completion clean up its descendants. There is no SSH execution
path or automatic execution retry.

Required service failure ends the VM. The host directly ends its own process to
stop libkrun; there is no guest shutdown RPC, reboot sequence or automatic sync.
Applications own flushing writes. Disconnect immediately kills an exec subtree.

Networking uses gvproxy with a fixed IPv4 address, route and DNS configuration.
Podman has a private Unix API socket;
the host reaches it through a dedicated vsock/Yamux proxy. Only explicit `--mount`
paths are shared. Persistent storage is managed by the host.

Build from the repository root with `go run ./scripts`. The nested
Go module uses the parent's protocol and execution packages through a local
module replacement.
