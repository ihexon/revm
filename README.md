# revm

[![build.yml](https://github.com/ihexon/revm/actions/workflows/build.yml/badge.svg)](https://github.com/ihexon/revm/actions/workflows/build.yml)

> [!WARNING] 
> This project is currently under heavy development, and any breaking changes may cause instability

[README_EN](./README.md) | [README_ZH](./README_zh.md)

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
wget https://github.com/ihexon/revm/releases/download/<TAG>/revm-Darwin-arm64.tar.zst

# Remove macOS quarantine attribute
xattr -d com.apple.quarantine revm-Darwin-arm64.tar.zst

# Extract
tar -xvf revm-Darwin-arm64.tar.zst
```

## Quick Start

### Run an ephemeral shell

Without `--workspace`, everything is ephemeral (similar to `docker run --rm`).

```bash
revm run sh             # start an ephemeral shell
revm run -- uname -a    # run a single command
```

### Launch a VM instance and attach to the guest shell

Use a long-running command (e.g. `sleep`) to keep the VM alive, then use `attach` from another terminal to open a shell
or run commands inside the guest.

```shell
export WORKSPACE=/tmp/my_work
revm run --workspace $WORKSPACE -- sleep 10000

# Interactive shell
revm attach --pty $WORKSPACE
# Non-interactive command
revm attach $WORKSPACE -- echo hello
```

### Launch the container engine

The Podman API socket is exposed at `$WORKSPACE/socks/podman-api.sock`. Point your `docker`/`podman` CLI at it by
setting the `CONTAINER_HOST` or `DOCKER_HOST` environment variable.

```shell
export WORKSPACE=/tmp/my_workspace
revm docker --log-level info --workspace $WORKSPACE

# In another terminal
export WORKSPACE=/tmp/my_workspace
export CONTAINER_HOST=unix:///$WORKSPACE/socks/podman-api.sock
podman info
podman run --rm -dit alpine:edge sh
```

## Commands

### `revm run` — Run a command in a Linux VM

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

**Example:**

```bash
revm run --workspace /tmp/my_workspace --rootfs ./my-ubuntu-rootfs bash
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

```shell
export WORKSPACE=/tmp/my_workspace
revm docker --log-level info --workspace $WORKSPACE

# In another terminal
export WORKSPACE=/tmp/my_workspace
export CONTAINER_HOST=unix:///$WORKSPACE/socks/podman-api.sock
podman info
podman run --rm -dit alpine:edge sh
```

### `revm attach` — Attach to a running VM

Opens an additional terminal session to a running VM instance via SSH. Useful for multi-terminal workflows.

```bash
revm attach [flags] <workspace-path> [-- command]
```

| Flag             | Description                                    | Default |
|------------------|------------------------------------------------|---------|
| `--pty`, `--tty` | Allocate a pseudo-terminal (interactive shell) | `false` |

**Examples:**

```bash
# Open an interactive shell
revm attach --pty ~/my_space

# Run a command non-interactively
revm attach ~/my_space -- ls -la /
```

### Global Flags

| Flag          | Description                                                                | Default |
|---------------|----------------------------------------------------------------------------|---------|
| `--log-level` | Log verbosity: `trace`, `debug`, `info`, `warn`, `error`, `fatal`, `panic` | `warn`  |

## Mount host directory to guest

Mount host directories into the guest via VirtIO-FS using the `--mount` flag.

**Format:** `host_path:guest_path[:ro]`

| Part         | Required | Description                                          |
|--------------|----------|------------------------------------------------------|
| `host_path`  | yes      | Absolute path on the host                            |
| `guest_path` | no       | Mount point inside the guest (defaults to host path) |
| `ro`         | no       | Mount as read-only (default is read-write)           |

```bash
# Read-write mount
revm run --mount /Users/me/projects:/mnt/projects sh

# Read-only mount
revm run --mount /Users/me/data:/mnt/data:ro sh

# Multiple mounts
revm run --mount /path/a:/mnt/a --mount /path/b:/mnt/b sh

# Omit guest path — mounts to the same path inside the guest
revm run --mount /Users/me/projects sh
```

## Mount raw disk to guest

Attach ext4 disk images using the `--raw-disk` flag. Each disk is automatically mounted at `/mnt/<UUID>` inside the
guest with `rw,discard` options.

- If the disk file does **not** exist, a 10 GB ext4 image is created and formatted automatically.
- Multiple disks can be attached by repeating the `--raw-disk` flag.

```bash
# Attach a single disk (auto-created if missing)
revm run --raw-disk ~/mydisk.ext4 sh

# Inside the guest
$ mount
/dev/vda on /mnt/1f803bc6-db09-48b7-96af-e027fd616afe type ext4 (rw,relatime,discard,data=ordered)

# Attach multiple disks
revm run --raw-disk ~/disk1.ext4 --raw-disk ~/disk2.ext4 sh
```

## Workspace

By default the guest filesystem is ephemeral (stored under `/tmp`). To persist data across runs, pass a stable
`--workspace` directory — all rootfs changes and SSH keys are saved there.

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

## Networking

### GVISOR (default)

revm uses [gvisor-tap-vsock](https://github.com/containers/gvisor-tap-vsock) as a userspace network stack. The guest
always gets IP `192.168.127.2`. You can reach TCP services on the host from inside the guest via the domain
`host.containers.internal`.

## Bug Reports

https://github.com/ihexon/revm/issues

## License

Apache License 2.0 — see [LICENSE](./LICENSE) for details.
