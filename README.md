# revm

`revm` is a lightweight Linux command-line runtime launcher that helps you quickly prepare a Linux testing/development environment.

You don’t need a full Linux UEFI image or to install a distribution from an ISO. Just provide a Linux rootfs or a statically compiled ELF program, and you can launch a securely isolated Linux shell in seconds.

In addition, `revm` can serve as an alternative to Docker Desktop/Orbstack — faster, lighter, and fully compatible with the existing Docker CLI ecosystem.

---

## ✨ Features

- ⚡ **Instant startup**: enter a Linux shell within a second
- 🧹 **Clean**: does not modify any configuration on the host
- 🐳 **Container mode**: 100% compatible with the Docker CLI ecosystem
- 📦 **Flexible execution**: run a full rootfs or directly run a single ELF program (similar to WSL on macOS)
- 💽 **Disk mounting**: supports mounting external image files (ext4/btrfs/xfs, etc.), automatically mounted at `/var/tmp/mnt/`
- 📂 **Directory mounting**: map host directories into the guest
- 🖥 **Multiple terminals**: attach to a running instance at any time

---

## 🚀 Quick Start

### Quick install
```shell
$ wget https://github.com/ihexon/revm/releases/download/latest/revm.tar 
$ tar -xvf revm.tar
$ ./out/bin/revm --help
```


### rootfs mode
```shell
# Download and extract Alpine rootfs
mkdir alpine_rootfs
wget -qO- https://dl-cdn.alpinelinux.org/alpine/v3.22/releases/aarch64/alpine-minirootfs-3.22.1-aarch64.tar.gz | tar -xv -C alpine_rootfs

# Start the isolated environment
revm rootfs-mode --rootfs alpine_rootfs -- /bin/sh

# Attach to a running instance
revm attach ./alpine_rootfs
```


### docker-mode
docker-mode requires a rootfs with Podman preinstalled. All containers will be stored in the image file specified by `--data-storage` (formatted as ext4).
you can get a podman preinstalled rootfs from [here](https://github.com/ihexon/prebuilds/raw/refs/heads/main/rootfs/arm64/alpine/rootfs.tar.zst).

It’s straightforward to use. Once the docker-engine is running, set the `CONTAINER_HOST` (for the podman CLI) or `DOCKER_HOST` (for the docker CLI) to `unix:///tmp/docker_api.sock` and use docker/podman commands as usual.

```shell
# get podman preinstalled rootfs
wget https://github.com/ihexon/prebuilds/raw/refs/heads/main/rootfs/arm64/alpine/rootfs.tar.zst 
tar -xvf rootfs.tar.zst
revm docker-mode --rootfs ~/rootfs --data-storage ~/data.disk

# Docker CLI
export DOCKER_HOST=unix:///tmp/docker_api.sock
docker info

# Podman CLI
export CONTAINER_HOST=unix:///tmp/docker_api.sock
podman system info
```


# ⚙️ Advanced Usage

## Mount image files into the guest
```textmate
# Automatically mount data1.disk and data2.disk inside the guest at /var/tmp/mnt/
revm rootfs-mode --rootfs alpine_rootfs \
  --data-disk ~/data1.disk \
  --data-disk ~/data2.disk \
  -- /bin/sh

# Logs
INFO[2025-09-09T17:34:27+08:00] mount "/Users/danhexon/data1.disk" -> "/var/tmp/mnt/Users/danhexon/data1.disk"
INFO[2025-09-09T17:34:27+08:00] mount "/Users/danhexon/data2.disk" -> "/var/tmp/mnt/Users/danhexon/data2.disk"
```


## Mount a host folder into the guest
```shell
# Mount /Users/danhexon from the host to /tmp/hostfs/danhexon inside the guest
revm rootfs-mode --rootfs alpine_rootfs --mount /Users/danhexon:/tmp/hostfs/danhexon -- /bin/sh
```


## Inherit the host’s proxy settings
Use `--system-proxy` to pass proxy settings into the guest:
```shell
revm rootfs-mode --rootfs alpine_rootfs --system-proxy -- /bin/sh
```


# BUG Reports
https://github.com/ihexon/revm/issues

# TODO
- [ ] Automatically configure a transparent proxy in the guest
- [ ] Support sending events to upper-layer applications