# revm

A lightweight Linux sandbox and container launcher for macOS, powered
by [libkrun](https://github.com/containers/libkrun). Boot a full Linux environment or a Podman container engine in under
1 second.

## Features

- **Sub-second startup** — launch a Linux shell almost instantly via libkrun's lightweight VM
- **Built-in Podman engine** — run containers with the standard `podman` CLI, no Docker Desktop required
- **Zero configuration** — ships with a bundled Alpine rootfs and kernel; works out of the box
- **Multi-terminal** — attach additional shells to a running VM via SSH over vsock
- **Directory sharing** — mount host directories into the guest via VirtIO-FS
- **Raw disk mounting** — attach ext4 disk images that are auto-mounted inside the guest
- **Persistent workspaces** — optionally preserve the rootfs and data across runs
- **Custom rootfs** — bring your own Linux rootfs (Ubuntu, Debian, etc.)
- **Network flexibility** — choose between gvisor userspace networking or TSI (Transparent Socket Interception)
- **System proxy passthrough** — forward host HTTP/HTTPS proxy settings into the guest

## Requirements

macOS Sonoma or later (Apple Silicon)

## Installation

```bash
# Download the latest release
wget https://github.com/ihexon/revm/releases/download/v4.1.4/revm-Darwin-arm64.tar.zst

# Remove macOS quarantine attribute
xattr -d com.apple.quarantine revm-Darwin-arm64.tar.zst

# Extract
tar -xvf revm-Darwin-arm64.tar.zst
```

## Quick Start

```bash
# Launch a Linux shell (uses built-in Alpine rootfs)
revm run sh

# Run a one-off command
revm run -- uname -a

# Launch with more resources
revm run --cpus 4 --memory 2048 sh
```

## Commands

### `revm run` — Run a Linux shell

Boots a lightweight VM with the built-in (or custom) rootfs and executes the given command.

```bash
revm run [flags] <command> [args...]
```

| Flag             | Description                                                    | Default               |
|------------------|----------------------------------------------------------------|-----------------------|
| `--cpus`         | Number of CPU cores                                            | `1`                   |
| `--memory`       | Memory in MB                                                   | `512`                 |
| `--workspace`    | Workspace directory for persistent state                       | `/tmp/.revm-<random>` |
| `--rootfs`       | Path to a custom rootfs directory                              | built-in Alpine       |
| `--workdir`      | Working directory inside the guest                             | `/`                   |
| `--raw-disk`     | Attach a raw disk image (repeatable)                           | —                     |
| `--mount`        | Mount a host directory (repeatable, format: `host:guest[:ro]`) | —                     |
| `--envs`         | Set environment variables (repeatable, format: `KEY=value`)    | —                     |
| `--network`      | Network stack: `GVISOR`                                        | `GVISOR`              |
| `--system-proxy` | Forward host HTTP/HTTPS proxy to guest                         | `false`               |

**Examples:**

```bash
# Run with a custom rootfs
revm run --rootfs ./my-ubuntu-rootfs bash

# Pass environment variables
revm run --envs FOO=bar --envs BAZ=qux sh

# Set working directory
revm run --workdir /home sh
```

### `revm container` — Launch the Podman container engine

Boots a VM with a built-in Podman service and exposes the API via a Unix socket on the host. Uses all available CPU
cores and memory by default.

```bash
revm container [flags]
```

| Flag             | Description                                | Default               |
|------------------|--------------------------------------------|-----------------------|
| `--cpus`         | Number of CPU cores                        | all cores             |
| `--memory`, `-m` | Memory in MB                               | all available         |
| `--workspace`    | Workspace directory                        | `/tmp/.revm-<random>` |
| `--raw-disk`     | Attach a raw disk image (repeatable)       | —                     |
| `--mount`        | Mount a host directory (repeatable)        | —                     |
| `--network`      | Network stack: `GVISOR` or `TSI`           | `GVISOR`              |
| `--system-proxy` | Forward host proxy to the container engine | `false`               |

**Example:**

```bash
$ revm container --log-level info
INFO Podman API proxy listen in: unix:///tmp/.revm-NI2kxCG2/socks/podman-api.sock

# In another terminal, point podman at the socket
$ export CONTAINER_HOST=unix:///tmp/.revm-NI2kxCG2/socks/podman-api.sock
$ podman run --rm alpine echo hello
hello
```

### `revm attach` — Attach to a running VM

Opens an additional terminal session to a running VM instance via SSH. Useful for multi-terminal workflows.

```bash
revm attach [flags] <workspace-path> [command]
```

| Flag             | Description                                    | Default |
|------------------|------------------------------------------------|---------|
| `--pty`, `--tty` | Allocate a pseudo-terminal (interactive shell) | `false` |

**Examples:**

```bash
# Open an interactive shell
revm attach --pty ~/my_space

# Run a command non-interactively
revm attach ~/my_space ls -la /
```

### Global Flags

| Flag           | Description                                                                | Default |
|----------------|----------------------------------------------------------------------------|---------|
| `--log-level`  | Log verbosity: `trace`, `debug`, `info`, `warn`, `error`, `fatal`, `panic` | `warn`  |
| `--report-url` | Endpoint to report VM events (e.g., `unix:///var/run/events.sock`)         | —       |

## Workspace

By default the guest filesystem is ephemeral (stored under `/tmp`). To persist data across runs, pass a stable workspace
directory — all rootfs changes and SSH keys are saved there.

```bash
$ revm run --workspace ~/my_space -- sh -c 'echo hello > /hello.txt'
$ revm run --workspace ~/my_space -- sh -c 'cat /hello.txt'
hello
```

The workspace directory contains:

- `rootfs/` — the guest root filesystem
- `ssh/` — auto-generated SSH keypair for host-guest communication
- `socks/` — Unix sockets (ignition, vmctl, podman API, network)
- `logs/` — VM log files
- `raw-disk/` — container storage disk (container mode)

## Directory Sharing

Mount host directories into the guest via VirtIO-FS:

```bash
# Read-write mount
revm run --mount /Users/me/projects:/mnt/projects sh

# Read-only mount
revm run --mount /Users/me/data:/mnt/data:ro sh

# Multiple mounts
revm run --mount /path/a:/mnt/a --mount /path/b:/mnt/b sh
```

## Raw Disk Mounting

Attach ext4 disk images that are automatically mounted at `/mnt/<UUID>` inside the guest:

```bash
$ revm run --raw-disk ~/mydisk.ext4 -- mount
/dev/vda on /mnt/1f803bc6-db09-48b7-96af-e027fd616afe type ext4 (rw,relatime,discard,data=ordered)
```

Multiple disks can be attached with repeated `--raw-disk` flags. If the disk file does not exist, it will be created and
formatted automatically.

## Networking

revm supports two network modes:

- GVISOR
- TSI (ToDo)

### GVISOR (default)

Uses [gvisor-tap-vsock](https://github.com/containers/gvisor-tap-vsock) as a userspace network stack. The guest always
gets IP `192.168.127.2`. You can reach TCP services on the host from inside the guest via the domain
`host.containers.internal`.

### TSI (Transparent Socket Impersonation)
TODO

## Bug Reports

https://github.com/ihexon/revm/issues

## License

Apache License 2.0 — see [LICENSE](./LICENSE) for details.
