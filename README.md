# revm

A lightweight Linux VM launcher for macOS, powered by [libkrun](https://github.com/containers/libkrun) and Apple's Hypervisor.framework. Launch a Linux shell in under a second.

## Features

- **Fast startup** - Enter a Linux shell in ~1 second, no heavy VM overhead
- **Zero configuration** - No system modifications, no daemons, just run
- **Docker/Podman compatible** - Full Docker CLI compatibility via built-in Podman
- **Two execution modes** - Rootfs mode for direct Linux execution, Container mode for Docker workflows
- **Disk mounting** - Mount virtual disk images (ext4/btrfs/xfs) into the guest
- **Directory sharing** - Share host directories with the guest via VirtIO-FS
- **Multi-terminal** - Attach multiple terminals to a running instance via SSH
- **Proxy passthrough** - Inherit host proxy settings in the guest

## Requirements

- macOS 13.1+ (Ventura or later)
- Apple Silicon (ARM64)

## Installation

```bash
# Download the latest release
wget https://github.com/ihexon/revm/releases/latest/download/revm.tar.zst

# Remove quarantine attribute (required for downloaded binaries)
xattr -d com.apple.quarantine revm.tar.zst

# Extract
tar -xvf revm.tar.zst

# Run
./out/bin/revm --help
```

## Quick Start

### Rootfs Mode

Run commands directly in a Linux rootfs:

```bash
# Download Alpine Linux rootfs
mkdir alpine && cd alpine
wget -qO- https://dl-cdn.alpinelinux.org/alpine/v3.21/releases/aarch64/alpine-minirootfs-3.21.3-aarch64.tar.gz | tar -xz
cd ..

# Launch a shell
revm rootfs-mode --rootfs ./alpine -- /bin/sh

# Or run a specific command
revm rootfs-mode --rootfs ./alpine -- /bin/echo "Hello from Linux!"
```

### Container Mode

Run Docker/Podman containers with persistent storage:

```bash
# Start container engine (creates data.disk if not exists)
revm docker-mode --data-storage ~/data.disk

# In another terminal, use Docker CLI
export DOCKER_HOST=unix:///tmp/docker_api.sock
docker run --rm alpine echo "Hello from container!"

# Or use Podman CLI
export CONTAINER_HOST=unix:///tmp/docker_api.sock
podman run --rm alpine echo "Hello from container!"
```

### Attach to Running Instance

```bash
# Attach a new terminal to a running VM
revm attach ./alpine -- /bin/sh

# Run a command in the running VM
revm attach ./alpine -- cat /etc/os-release
```

## Advanced Usage

### Mount Disk Images

Mount ext4/btrfs/xfs disk images into the guest:

```bash
# Create a disk image (if needed)
truncate -s 10G data.disk

# Mount disk images (auto-mounted at /var/tmp/mnt/...)
revm rootfs-mode --rootfs ./alpine \
  --data-disk ~/data1.disk \
  --data-disk ~/data2.disk \
  -- /bin/sh
```

### Share Host Directories

Mount host directories into the guest via VirtIO-FS:

```bash
# Mount host directory to guest path
revm rootfs-mode --rootfs ./alpine \
  --mount /Users/me/projects:/mnt/projects \
  -- /bin/sh
```

### Proxy Passthrough

Inherit the host's HTTP/HTTPS proxy settings:

```bash
revm rootfs-mode --rootfs ./alpine --system-proxy -- /bin/sh
```

### Resource Configuration

```bash
# Customize CPU and memory
revm rootfs-mode --rootfs ./alpine \
  --cpus 4 \
  --memory 4096 \
  -- /bin/sh
```

## Bug Reports

https://github.com/ihexon/revm/issues

## License

See [LICENSE](./LICENSE) for details.
