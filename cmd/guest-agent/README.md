# Guest Agent

`cmd/guest-agent` is the in-guest bootstrap process. Users should not run it directly; the host-side `revm` binary starts the VM, injects configuration, and lets the guest agent finish initialization.

## Responsibilities

- Extract embedded BusyBox and Dropbear helpers into `/.bin`.
- Fetch VM configuration from the host through vsock and persist it inside the guest.
- Mount `/proc`, `/sys`, `/dev`, `/tmp`, `/run`, raw block devices, and VirtIO-FS shares.
- Configure guest networking for `gvisor` or `tsi`.
- Start SSH, time sync, and mode-specific long-running services.
- Execute the user command for `revm run`.
- Start the Podman API service for `revm dockerd`.
- Configure Podman port publishing so container `-p` mappings call gvproxy expose/unexpose.
- Report readiness and lifecycle state back to the host.
- Sync disks and force reboot when the host requests shutdown.

## Boot Flow

1. Initialize logging and unpack embedded helper binaries.
2. Read the machine config from the host.
3. Mount pseudo filesystems, block devices, and shared directories.
4. Configure network.
5. Start SSH and time sync.
6. Dispatch by run mode:
   - `rootfs`: run the configured command.
   - `docker`: start the Podman API service and keep the VM alive.
7. Run readiness probes and notify the host.
8. Wait for shutdown, sync disks, and reboot.

## Host Control

The guest agent exposes no public user CLI. Host control flows through services created by the main `revm` process:

- `revm attach` obtains SSH metadata from the host management API and connects to Dropbear.
- `revm ctl --port-export` and `revm ctl --port-unexport` obtain the gvproxy endpoint from the host management API and call gvproxy's forwarder API.
- Container port publishing is initiated by Podman inside the guest and handled by gvproxy on the host.

## File Map

| Path | Purpose |
| ---- | ------- |
| `main.go` | Guest boot orchestration, mode dispatch, and lifecycle |
| `pkg/service/embedded.go` | Embedded BusyBox and Dropbear extraction |
| `pkg/service/mount.go` | Pseudo filesystem, block device, and VirtIO-FS mounts |
| `pkg/service/network.go` | Guest network setup for `gvisor` and `tsi` |
| `pkg/service/dropbear.go` | Dropbear SSH server bootstrap |
| `pkg/service/podman.go` | Podman system service bootstrap |
| `pkg/service/runcmdline.go` | User command execution with console handling |
| `pkg/service/readiness.go` | SSH, Podman, and network readiness probes |
| `pkg/service/shutdown.go` | Shutdown coordination |
| `pkg/supervisor/supervisor.go` | Minimal restart-capable process supervisor |
