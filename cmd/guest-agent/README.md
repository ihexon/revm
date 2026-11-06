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

Gvisor networking uses gvproxy with a fixed IPv4 address, route and DNS configuration.
TSI uses host IPv4/IPv6 sockets without a virtual interface or gvproxy; the guest
writes the host resolver snapshot from its boot configuration to /etc/resolv.conf.
Unix sockets stay guest-local. Podman requires gvisor.
Podman has a private Unix API socket. Each host client connection owns one
vsock transport, with no pooling or multiplexing. In each direction, a four-byte
big-endian length precedes at most 32 KiB of data; length zero is input EOF.
Reading never writes acknowledgments or window updates. The underlying socket
supplies backpressure. EOF preserves the response direction; copy errors or
cancellation close both endpoints. Partial or oversized frames fail the connection.
This internal protocol requires the host and guest agent from the same bundle;
there is no legacy-protocol fallback. Only explicit `--mount` paths are shared.
Persistent storage is managed by the host.

Build from the repository root with `go run ./scripts`. The nested
Go module uses the parent's protocol and execution packages through a local
module replacement.
