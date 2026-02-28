# Workspace & Networking

## Workspace Layout

Files inside the `--workspace` directory:

```
$WORKSPACE/
├── socks/
│   ├── podman-api.sock   # Podman API socket (docker mode)
│   ├── gvpctl.sock       # gvproxy control socket (gvisor mode)
│   ├── vnet.sock         # virtual network socket (gvisor mode)
│   ├── vmctl.sock        # VM management API socket
│   └── ign.sock          # ignition config service socket
├── ssh/
│   └── private.key       # auto-generated SSH private key
├── logs/
│   └── vm.log            # VM internal logs
├── rootfs/               # guest root filesystem (chroot mode)
└── raw-disk/             # container storage disk (docker mode)
```

### Reuse & Cleanup

**Reuse**: pass the same `--workspace` path on the next launch and the VM picks up exactly where it left off —
container images, volume data, rootfs, and SSH keys are all preserved; no reconfiguration or re-pulling required:

```bash
# First launch
revm docker --workspace ~/revm_workspace

# Next launch — images and data are still there
revm docker --workspace ~/revm_workspace
```

**Ephemeral environment**: when `--workspace` is omitted, revm uses a random directory under `/tmp`. It is safe to
delete after the VM exits, making it ideal for one-off tasks or CI pipelines.

**Cleanup**: delete the workspace directory to completely reset; the next launch starts fresh:

```bash
rm -rf ~/revm_workspace
```

## Networking (TSI / GVISOR, mutually exclusive)

Both docker mode and chroot mode support two network backends. They are mutually exclusive.

### gvisor (default)

Uses [gvisor-tap-vsock](https://github.com/containers/gvisor-tap-vsock) as a userspace network stack. The guest always
gets IP `192.168.127.2` with gateway `192.168.127.1`. Services on the host are reachable inside the guest and
containers via `host.containers.internal`.

### tsi (Transparent Socket Interception)

TSI (Transparent Socket Impersonation) is a networking mode built into libkrun. **No virtual NIC is created** — the
guest and host share the network directly, and can access each other's TCP/UDP services without special IPs or
port-forwarding rules.

Compared to gvisor: **`-p` port mapping is not supported** (no gvproxy), and `host.containers.internal` is not
available. To expose container ports to macOS when using TSI, run containers with `podman run --network=host` to share
the host network directly; ports are then accessible on macOS without any additional mapping.
