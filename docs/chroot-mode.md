# chroot mode — Linux chroot alternative on macOS

revm's chroot mode boots a real Linux kernel to execute binaries inside a rootfs. Backed by libkrun + Apple
Hypervisor, **startup is typically under 1–2 seconds**, with near-native performance and stronger isolation. Jump into
any Linux rootfs directly from macOS.

## Common Use Cases

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

## Flags

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
| `--log-level`    | Log verbosity: `trace`, `debug`, `info`, `warn`, `error`, `fatal`, `panic`          | `info`                |
| `--report-url`   | HTTP endpoint to receive VM lifecycle events (e.g. `unix:///var/run/events.sock`)   | —                     |

## See Also

- [Workspace layout & networking](insider.md) — workspace directory structure, reuse/cleanup, and network backends (gvisor / tsi)
