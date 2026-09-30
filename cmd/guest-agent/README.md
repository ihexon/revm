# Guest agent

cmd/guest-agent builds the process injected into the Alpine VM at /.bin/guest-agent. It is an internal component; users normally interact with it through revm run, revm dockerd, revm attach, or revm ctl.

## Responsibilities

The guest agent:

- reads the VM configuration delivered through the ignition vsock or Unix socket;
- mounts /proc, /sys, /dev, /tmp, /run, block devices, and VirtIO-FS shares;
- configures gvisor or tsi networking;
- starts the guest-control HTTP-over-vsock service;
- starts the optional dropbear SSH compatibility endpoint;
- runs the command supplied to revm run;
- starts the Podman API service for revm dockerd;
- supervises the Podman and dropbear processes;
- flushes mounted disks and reboots the guest when the compatibility shutdown path is used.

The Alpine rootfs is built separately and contains the runtime commands and apk packages. The guest agent is the only executable injected at runtime.

## Boot sequence

1. Initialize logging from LOG_LEVEL.
2. Read the guest specification from the host.
3. Mount pseudo filesystems and the configured storage.
4. Attach the guest log virtio port when available.
5. Start the guest-control server.
6. Configure the selected network.
7. Dispatch by mode:
   - command mode runs the requested binary and exits when it finishes;
   - container mode starts Podman system service and remains alive.
8. Start time synchronization and the SSH compatibility endpoint.
9. Report readiness through the host-side management and Podman probes.
10. On shutdown, stop child services, sync disks, and reboot.

The host normally requests shutdown through libkrun's native shutdown API. Guest-control POST /v1/shutdown remains the compatibility fallback for guests or older backends that do not complete native shutdown.

## Guest-control

The host-side attach path uses the guest-control endpoint for non-PTY commands. The endpoint streams stdout, stderr, and the final exit status. The shutdown endpoint is intentionally small and is not a general-purpose guest management API.

PTY support is separate. revm attach --pty uses the SSH compatibility service because terminal allocation and interactive terminal resize are not part of the guest-control stream.

## Storage and ownership

The agent mounts VirtIO-FS shares at the configured target paths and mounts block devices at their configured guest paths. Existing shared inodes retain the host UID, GID, and mode as reported by libkrun passthrough.

## Networking

gvisor mode obtains an address through gvisor-tap-vsock and configures the guest route and DNS. tsi mode uses libkrun transparent socket interception and does not start gvproxy. Container mode is intentionally restricted to gvisor because Podman API proxying and container port publication use gvproxy.

## Source map

| Path | Responsibility |
| --- | --- |
| main.go | boot orchestration and mode dispatch |
| pkg/service/vmconfig.go | fetch and decode the host VM configuration |
| pkg/service/mount.go | pseudo filesystems, block devices, and VirtIO-FS |
| pkg/service/network.go | gvisor and tsi guest networking |
| pkg/service/guestcontrol.go | command and shutdown HTTP service |
| pkg/service/dropbear.go | SSH compatibility service |
| pkg/service/podman.go | Podman system service |
| pkg/service/runcmdline.go | command execution and output handling |
| pkg/service/shutdown.go | compatibility sync and reboot path |
| pkg/supervisor/supervisor.go | restart-capable child process supervisor |
| pkg/network and pkg/vsock | guest network and vsock helpers |

## Build

The guest agent is built by the root build script and embedded into the host release. Build the application from the repository root:

~~~bash
go run ./scripts --build revm
~~~

The guest agent depends on the protocol and define packages from the parent module. Keep changes to its wire format in sync with the host-side service.
