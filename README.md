<h1 align="center"><b>revm</b></h1>
<p align="center">revm helps you quickly launch Linux VMs / Containers</p>

[![build.yml](https://github.com/ihexon/revm/actions/workflows/build.yml/badge.svg)](https://github.com/ihexon/revm/actions/workflows/build.yml)

> [!WARNING]
> This project is currently under heavy development

[README_EN](./README.md) | [README_ZH](./README_zh.md)

A lightweight Linux microVM for macOS powered by [libkrun](https://github.com/containers/libkrun). Two independent
modes: **chroot mode** (run commands inside an isolated Linux rootfs) and **docker mode** (run a full Podman container
engine on Apple Silicon).

## Requirements

macOS Sonoma or later

## Installation

```bash
wget https://github.com/ihexon/revm/releases/download/<TAG>/revm-Darwin-arm64.tar.zst

xattr -d com.apple.quarantine revm-Darwin-arm64.tar.zst

tar -xvf revm-Darwin-arm64.tar.zst
```

---

## chroot mode — Linux chroot alternative on macOS

revm's chroot mode boots a real Linux kernel to execute binaries inside a rootfs. Backed by libkrun + Apple
Hypervisor, **startup is typically under 1–2 seconds**, with near-native performance and stronger isolation. Jump into
any Linux rootfs directly from macOS.

### Common Use Cases

**Run any rootfs**

```bash
# Integration test with your own Ubuntu rootfs
revm chroot --rootfs ~/ubuntu-jammy -- bash -c 'apt-get install -y libssl-dev && make test'

# Just need a quick Linux shell — use the built-in Alpine rootfs
revm chroot -- sh -c 'uname -r'
```

**Mount a host source directory for compilation**

```bash
revm chroot \
  --rootfs ~/ubuntu-rootfs \
  --mount /Users/me/myproject:/workspace \
  --workdir /workspace \
  bash -c 'make && ./run_tests.sh'
```

**Keep the VM alive and attach interactively when needed**

```bash
export WORKSPACE=/tmp/dev-env

# Terminal 1: keep the VM alive
revm chroot --workspace $WORKSPACE --rootfs ~/ubuntu-rootfs sleep 86400

# Terminal 2: open an interactive shell
revm attach --pty $WORKSPACE

# Terminal 3: run a one-off command
revm attach $WORKSPACE -- df -h
```

**Attach a persistent data disk**

```bash
# First run: auto-creates an ext4 image, mounted at /mnt/<UUID> inside the guest
revm chroot --raw-disk ~/data.ext4 sh -c 'mount'

# Subsequent runs: reuse the same disk, data persists
revm chroot --raw-disk ~/data.ext4 sh -c 'ls /mnt'
```

### Flags

```bash
revm chroot [flags] <command> [args...]
```

| Flag             | Description                                                                         | Default               |
|------------------|-------------------------------------------------------------------------------------|-----------------------|
| `--rootfs`       | Path to a rootfs directory; must contain `/bin/sh`; falls back to built-in Alpine   | built-in Alpine       |
| `--cpus`         | Number of vCPU cores; defaults to host CPU count if unset or less than 1            | host CPU count        |
| `--memory`       | VM memory in MB; minimum 512 MB; defaults to host available memory if unset         | host available memory |
| `--workdir`      | Working directory inside the guest before running the command                       | `/`                   |
| `--mount`        | Share a host directory via VirtIO-FS (format: `/host:/guest[:ro]`; repeatable)      | —                     |
| `--raw-disk`     | Attach an ext4 disk image; auto-created as 10 GB if missing (repeatable)            | —                     |
| `--envs`         | Pass environment variables (format: `KEY=VALUE`; repeatable)                        | —                     |
| `--network`      | Network stack: `gvisor` (full virtual NIC) or `tsi` (transparent socket intercept)  | `gvisor`              |
| `--system-proxy` | Read macOS system proxy and inject as `http_proxy`/`https_proxy` into the VM        | `false`               |
| `--workspace`    | Runtime state directory (sockets, SSH keys, logs, disks); ephemeral if unset        | `/tmp/.revm-<random>` |

---

## docker mode — full container engine without Docker Desktop

revm embeds a complete container engine and exposes it via a Unix socket to `podman`/`docker` CLI. No Docker Desktop
or Podman Desktop required — spin up a full, lightweight container stack instantly.

### Quick Start

**Start the container engine**

```bash
export WORKSPACE=~/revm_workspace
revm docker --workspace $WORKSPACE
```

After startup, the Podman API socket is available at `$WORKSPACE/socks/podman-api.sock`.

**Connect with podman or docker CLI**

```bash
export CONTAINER_HOST=unix:///$WORKSPACE/socks/podman-api.sock

# Check runtime info
podman info

# Run containers (fully Docker-compatible)
podman run --rm ubuntu:latest uname -r
podman run --rm -it alpine:edge sh
podman run --rm -p 8080:80 nginx
```

**Connect with docker CLI**

```bash
export DOCKER_HOST=unix:///$WORKSPACE/socks/podman-api.sock
docker run --rm hello-world
```

### Port Mapping

In docker mode, container port mappings (`-p`) are automatically forwarded to macOS via gvproxy:

```bash
podman run --rm -p 8888:80 nginx
# Access directly on macOS
curl http://127.0.0.1:8888
```

### Mount Host Directories

```bash
podman run --rm -v /Users/me/data:/data ubuntu:latest ls /data
```

### System Proxy Passthrough

In environments that require a proxy, add `--system-proxy` to automatically read the macOS system proxy and inject it
into containers:

```bash
revm docker --workspace $WORKSPACE --system-proxy

# apt/curl inside containers automatically use the proxy
podman run --rm ubuntu:latest apt-get update
```

### Flags

```bash
revm docker [flags]
```

| Flag             | Description                                                                                         | Default               |
|------------------|-----------------------------------------------------------------------------------------------------|-----------------------|
| `--cpus`         | Number of vCPU cores; defaults to host CPU count if unset or less than 1                            | host CPU count        |
| `--memory`       | VM memory in MB; minimum 512 MB; defaults to host available memory if unset                         | host available memory |
| `--mount`        | Share a host directory via VirtIO-FS (format: `/host:/guest[:ro]`; repeatable)                      | —                     |
| `--raw-disk`     | Attach an ext4 disk image; auto-created if missing (repeatable)                                     | —                     |
| `--network`      | Network stack: `gvisor` (full virtual NIC, supports port mapping) or `tsi` (transparent intercept)  | `gvisor`              |
| `--system-proxy` | Read macOS system proxy and inject into containers; rewrites `127.0.0.1` to `host.containers.internal` | `false`            |
| `--workspace`    | Runtime state directory; Podman API socket at `socks/podman-api.sock` inside this directory         | `/tmp/.revm-<random>` |

docker mode and chroot mode share most flags and can be configured as needed.

---

## revm attach — connect to a running VM

Attach to a running VM instance from another terminal.

```bash
revm attach [--pty] <workspace> [-- <command> [args...]]
```

| Flag    | Description                                                                                                      | Default |
|---------|------------------------------------------------------------------------------------------------------------------|---------|
| `--pty` | Allocate a pseudo-terminal and launch an interactive shell; without this flag the command runs non-interactively | `false` |

```bash
# Interactive shell
revm attach --pty ~/revm_workspace

# Run a single command
revm attach ~/revm_workspace -- ps aux
```

---

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

## Bug Reports

https://github.com/ihexon/revm/issues

## License

Apache License 2.0 — see [LICENSE](./LICENSE) for details.

> Some parts of this document were written using AI assistance because I was lazy.