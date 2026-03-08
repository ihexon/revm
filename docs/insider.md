# Session Directory & Networking

## Session Directory Layout

Each session has a directory at `/tmp/<id>`, derived from the `--id` flag (or a random string if omitted):

```
/tmp/<id>/
├── socks/
│   ├── podman-api.sock   # Podman API socket (docker mode)
│   ├── gvpctl.sock       # gvproxy control socket (gvisor mode)
│   ├── vnet.sock         # virtual network socket (gvisor mode)
│   ├── vmctl.sock        # VM management API socket
│   └── ign.sock          # ignition config service socket
├── ssh/
│   ├── key               # auto-generated SSH private key
│   └── key.pub           # auto-generated SSH public key
├── logs/
│   └── vm.log            # VM internal logs
├── rootfs/               # guest root filesystem (chroot mode)
└── raw-disk/             # container storage disk (docker mode)
```

### Symlink Flags

Several flags create symbolic links pointing into the session directory, so external tools can find resources at
a well-known path without breaking session directory integrity:

| Flag               | Symlink target                                  |
|--------------------|-------------------------------------------------|
| `--podman-proxy-api-file` | `<session_dir>/socks/podman-api.sock`    |
| `--manage-api-file` | `<session_dir>/socks/vmctl.sock`                |
| `--ssh-key-dir`    | `<session_dir>/ssh/key` and `<session_dir>/ssh/key.pub` |
| `--export-ssh-private-key` | `<session_dir>/ssh/key`                 |
| `--export-ssh-public-key`  | `<session_dir>/ssh/key.pub`             |

### Session Lifecycle

The session directory is **ephemeral** — after the VM exits, `/tmp/<id>/` is automatically removed during cleanup.
Each launch starts with a fresh directory.

**Mutual exclusion**: sessions with the same `--id` are mutually exclusive via flock — only one VM can use a given
ID at a time. This makes `--id` useful for `revm attach` to connect to a running session.

**Persistent data**: to keep data across sessions, use explicit flags that point outside the session directory:

```bash
# Container images survive across sessions
revm docker --id my-engine --container-disk ~/container-storage.ext4

# Arbitrary data persists too
revm chroot --raw-disk ~/data.ext4 -- sh
```

**Cleanup**: if the process was forcefully killed (e.g. `kill -9`), manually remove the stale session directory:

```bash
rm -rf /tmp/my-engine
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
