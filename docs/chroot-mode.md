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
# Terminal 1: keep the VM alive
revm chroot --id dev-env --rootfs ~/ubuntu-rootfs sleep 86400

# Terminal 2: open an interactive shell
revm attach --pty dev-env

# Terminal 3: run a one-off command
revm attach dev-env -- df -h
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

| Flag               | Description                                                                         | Default               |
|--------------------|-------------------------------------------------------------------------------------|-----------------------|
| `--rootfs`         | Path to a rootfs directory; must contain `/bin/sh`; falls back to built-in Alpine   | built-in Alpine       |
| `--id`             | Session ID; session directory is derived as `/tmp/<id>`; defaults to a random string; sessions with the same ID are mutually exclusive via flock | random |
| `--cpus`           | Number of vCPU cores; defaults to host CPU count if unset or less than 1            | host CPU count        |
| `--memory`         | VM memory in MB; minimum 512 MB; defaults to host available memory if unset         | host available memory |
| `--workdir`        | Working directory inside the guest before running the command                       | `/`                   |
| `--mount`          | Share a host directory via VirtIO-FS (format: `/host:/guest[,ro]`; repeatable)      | —                     |
| `--raw-disk`       | Attach an ext4 disk image; auto-created as 10 GB if missing (repeatable)            | —                     |
| `--envs`           | Pass environment variables (format: `KEY=VALUE`; repeatable)                        | —                     |
| `--network`        | Network stack: `gvisor` (full virtual NIC) or `tsi` (transparent socket intercept)  | `gvisor`              |
| `--system-proxy`   | Read macOS system proxy and inject as `http_proxy`/`https_proxy` into the VM        | `false`               |
| `--manage-api`     | Symlink path for the VM management API socket; the actual socket is always inside the session directory | — |
| `--ssh-key-dir`    | Directory to symlink the generated SSH key pair (`key` and `key.pub`) into; keys are always created inside the session directory | — |
| `--log-level`      | Log verbosity: `trace`, `debug`, `info`, `warn`, `error`, `fatal`, `panic`          | `info`                |
| `--log-to`         | Custom log file path on host; defaults to `<session_dir>/logs/vm.log`               | session-local         |
| `--report-url`     | HTTP endpoint to receive VM lifecycle events (e.g. `unix:///var/run/events.sock` or `tcp://host:port`) | — |

## See Also

- [Session workspace & networking](insider.md) — session directory structure, reuse/cleanup, and network backends (gvisor / tsi)
