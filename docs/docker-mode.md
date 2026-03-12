# docker mode — full container engine without Docker Desktop

revm embeds a complete container engine and exposes it via a Unix socket to `podman`/`docker` CLI. No Docker Desktop
or Podman Desktop required — spin up a full, lightweight container stack instantly.

## Quick Start

**Start the container engine**

```bash
revm docker --id my-engine
```

After startup, the Podman API socket is available at `/tmp/my-engine/socks/podman-api.sock`.

**Connect with podman or docker CLI**

```bash
export CONTAINER_HOST=unix:///tmp/my-engine/socks/podman-api.sock

# Check runtime info
podman info

# Run containers (fully Docker-compatible)
podman run --rm ubuntu:latest uname -r
podman run --rm -it alpine:edge sh
podman run --rm -p 8080:80 nginx
```

**Connect with docker CLI**

```bash
export DOCKER_HOST=unix:///tmp/my-engine/socks/podman-api.sock
docker run --rm hello-world
```

## Port Mapping

In docker mode, container port mappings (`-p`) are automatically forwarded to macOS via gvproxy:

```bash
podman run --rm -p 8888:80 nginx
# Access directly on macOS
curl http://127.0.0.1:8888
```

## Mount Host Directories

```bash
podman run --rm -v /Users/me/data:/data ubuntu:latest ls /data
```

## System Proxy Passthrough

In environments that require a proxy, add `--system-proxy` to automatically read the macOS system proxy and inject it
into containers:

```bash
revm docker --id my-engine --system-proxy

# apt/curl inside containers automatically use the proxy
podman run --rm ubuntu:latest apt-get update
```

## Flags

```bash
revm docker [flags]
```

| Flag               | Description                                                                                         | Default               |
|--------------------|-----------------------------------------------------------------------------------------------------|-----------------------|
| `--id`             | **Required.** Session ID; session directory is derived as `/tmp/<id>`; sessions with the same ID are mutually exclusive via flock | — |
| `--cpus`           | Number of vCPU cores; defaults to host CPU count if unset or less than 1                            | host CPU count        |
| `--memory`         | VM memory in MB; minimum 512 MB; defaults to host available memory if unset                         | host available memory |
| `--envs`           | Pass environment variables (format: `KEY=VALUE`; repeatable)                                        | —                     |
| `--mount`          | Share a host directory via VirtIO-FS (format: `/host:/guest[,ro]`; repeatable)                      | —                     |
| `--raw-disk`       | Attach an ext4 disk image; auto-created if missing (repeatable)                                     | —                     |
| `--network`        | Network stack: `gvisor` (full virtual NIC, supports port mapping) or `tsi` (transparent intercept)  | `gvisor`              |
| `--system-proxy`   | Read macOS system proxy and inject into containers; rewrites `127.0.0.1` to `host.containers.internal` | `false`            |
| `--container-disk` | Path to a persistent ext4 raw disk image for container storage; auto-created if missing; defaults to a session-local disk | session-local |
| `--podman-proxy-api-file` | Custom Unix socket path for the Podman API proxy; defaults to `<session_dir>/socks/podman-api.sock` | —                  |
| `--manage-api-file` | Custom Unix socket path for the VM management API; defaults to `<session_dir>/socks/vmctl.sock` | —                  |
| `--ssh-key-dir`    | Directory to symlink the generated SSH key pair (`key` and `key.pub`) into; keys are always created inside the session directory | — |
| `--export-ssh-private-key` | File path to symlink the generated SSH private key to                        | —                     |
| `--export-ssh-public-key`  | File path to symlink the generated SSH public key to                         | —                     |
| `--log-level`      | Log verbosity: `trace`, `debug`, `info`, `warn`, `error`, `fatal`, `panic`                          | `info`                |
| `--log-to`         | Custom log file path on host; defaults to `<session_dir>/logs/vm.log`                               | session-local         |
| `--report-events-to` | HTTP endpoint to receive VM lifecycle events (e.g. `unix:///var/run/events.sock` or `tcp://host:port`) | —                  |

docker mode and chroot mode share most flags and can be configured as needed.

## See Also

- [Session workspace & networking](insider.md) — session directory structure, reuse/cleanup, and network backends (gvisor / tsi)
