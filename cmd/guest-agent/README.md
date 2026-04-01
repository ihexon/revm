# Guest Agent

`cmd/guest-agent` runs inside every `revm` VM as the in-guest bootstrap companion. It is not the normal user entry
point; the host-side `cmd/revm` process creates the VM, injects configuration, and this agent finishes guest
initialization.

## Responsibilities

- Extract embedded BusyBox and Dropbear binaries into `/.bin`.
- Fetch the VM configuration from the host over vsock and persist it inside the guest.
- Mount pseudo filesystems, raw block devices, and VirtIO-FS shares.
- Configure guest networking for `gvisor` or `tsi`.
- Start long-lived services such as SSH, Podman API, and NTP sync.
- Run the user command in `chroot` mode, or keep the container engine alive in `dockerd` mode.
- Probe readiness and report SSH / Podman / network status back to the host.
- Sync disks and power off the VM on shutdown signals.

## Boot Flow

1. Initialize logging and extract embedded helper binaries.
2. Read the machine config from the host via vsock.
3. Mount `/proc`, `/sys`, `/dev`, `/tmp`, `/run`, block devices, and VirtIO-FS mounts.
4. Start mode-specific services:
   - `chroot`: configure network, start SSH and time sync, then execute the requested command.
   - `dockerd`: configure network, start Podman API, SSH, and time sync.
5. Run readiness probes and send ready events back to the host.
6. Wait for shutdown, sync disks, and force power off the VM.

## File Map

| Path | Purpose |
|------|---------|
| `main.go` | Main orchestration for guest boot, mode dispatch, and lifecycle |
| `pkg/service/embedded.go` | Extract embedded BusyBox / Dropbear binaries |
| `pkg/service/mount.go` | Mount pseudo filesystems, block devices, and VirtIO-FS shares |
| `pkg/service/network.go` | Guest network setup for `gvisor` and `tsi` |
| `pkg/service/dropbear.go` | Dropbear SSH server bootstrap |
| `pkg/service/podman.go` | Podman system service bootstrap |
| `pkg/service/runcmdline.go` | Execute the user command, including TTY-aware console handling |
| `pkg/service/readiness.go` | SSH / Podman / interface readiness probes |
| `pkg/service/shutdown.go` | Shutdown coordination: sync then `poweroff -f` |
| `pkg/supervisor/supervisor.go` | Minimal restart-capable process supervisor used by guest services |
