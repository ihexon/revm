# revm

English | [中文](./README_zh.md) 


`revm` helps you quickly launch Linux VMs / Containers, lightning fast

---

## ✨ Features

- ⚡ **Lightweight**: enter a Linux shell within a second, start container engine (podman) in one second
- 🧹 **Clean**: does not modify any configuration on your machine
- 🐳 **Container compatible**: 100% compatible with the Docker CLI ecosystem
- 📦 **Flexible execution**: Rootfs mode and Container mode
- 💽 **Disk mounting**: automatically mount external virtual disk files (multiple formats: ext4/btrfs/xfs)
- 📂 **Directory mounting**: supports mounting host files into the VM
- 🖥 **Multiple terminals**: attach to running instances at any time to execute any command

---

## 🚀 Quick Start

### Quick install
```shell
$ wget https://github.com/ihexon/revm/releases/latest/download/revm.tar.zst
$ tar -xvf revm.tar.zst
$ ./out/bin/revm --help # help message
```

### Container mode
Container mode requires specifying an image file as the container storage area. Use `--data-storage` to reuse & generate image files (ext4 format)
```shell
revm docker-mode --data-storage ~/data.disk
```

Set the `CONTAINER_HOST` variable (used by podman CLI) or `DOCKER_HOST` (used by docker CLI) to `unix:///tmp/docker_api.sock` to use docker/podman CLI commands.

```shell
# Docker CLI 
export DOCKER_HOST=unix:///tmp/docker_api.sock
docker info

# Podman CLI
export CONTAINER_HOST=unix:///tmp/docker_api.sock 
podman system info
```

### rootfs mode

Quickly run any program in rootfs
```bash
# Download and extract Alpine rootfs
mkdir alpine_rootfs
wget -qO- https://dl-cdn.alpinelinux.org/alpine/v3.22/releases/aarch64/alpine-minirootfs-3.22.1-aarch64.tar.gz | tar -xv -C alpine_rootfs

# Start the isolated environment
revm rootfs-mode --rootfs alpine_rootfs -- /bin/sh

# Attach to a running instance
revm attach ./alpine_rootfs
```

# ⚙️ Advanced Usage

## Mount image files into the guest
```shell
# Automatically mount data1.disk and data2.disk inside the guest at /var/tmp/mnt/
revm rootfs-mode --rootfs alpine_rootfs \
  --data-disk ~/data1.disk \
  --data-disk ~/data2.disk \
  -- /bin/sh

# Log output
INFO[2025-09-09T17:34:27+08:00] mount "/Users/danhexon/data1.disk" -> "/var/tmp/mnt/Users/danhexon/data1.disk"
INFO[2025-09-09T17:34:27+08:00] mount "/Users/danhexon/data2.disk" -> "/var/tmp/mnt/Users/danhexon/data2.disk"
```

## Mount host folder into the guest
```shell
# Mount /Users/danhexon from the host to /tmp/hostfs/danhexon inside the guest
revm rootfs-mode --rootfs alpine_rootfs --mount /Users/danhexon:/tmp/hostfs/danhexon -- /bin/sh
```


## Inherit the host's proxy settings
Use `--system-proxy` to pass proxy settings into the guest:
```shell
revm rootfs-mode --rootfs alpine_rootfs --system-proxy -- /bin/sh
```

# BUG Reports
https://github.com/ihexon/revm/issues