# revm

`revm` 是一个 **轻量级 Linux 命令行运行环境启动器**，帮助你快速准备 Linux 测试 / 开发环境。

你无需完整的 Linux UEFI 镜像，也不需要从 ISO 安装发行版，只需准备一个 **Linux rootfs** 或一个 **静态编译的 ELF 程序**，即可秒级启动一个安全隔离的 Linux shell。

此外，`revm` 还能作为 **Docker Desktop / Orbstack 的替代品** —— 更快、更轻，并完全兼容现有的 Docker 命令行生态。

---

## ✨ 特性

- ⚡ **秒级启动**：一秒内进入 Linux shell
- 🧹 **干净**：不会修改宿主机的任何配置
- 🐳 **容器模式**：100% 兼容 Docker 命令行生态
- 📦 **灵活运行**：既能运行完整 rootfs，也能直接运行单个 ELF 程序（类似 macOS 上的 WSL）
- 💽 **磁盘挂载**：支持挂载外部镜像文件（ext4/btrfs/xfs 等），自动挂载到 `/var/tmp/mnt/`
- 📂 **目录挂载**：支持将宿主机目录映射到 guest 中
- 🖥 **多终端支持**：可随时 attach 到已运行的实例

---

## 🚀 快速开始

### 快速安装
```shell
$ wget https://github.com/ihexon/revm/releases/latest/download/revm.tar.zst
$ tar -xvf revm.tar.zst
$ ./out/bin/revm --help
```

### rootfs 模式

快速运行 rootfs 中的任何程序
```bash
# 下载并解压 Alpine rootfs
mkdir alpine_rootfs
wget -qO- https://dl-cdn.alpinelinux.org/alpine/v3.22/releases/aarch64/alpine-minirootfs-3.22.1-aarch64.tar.gz | tar -xv -C alpine_rootfs

# 启动隔离环境
revm rootfs-mode --rootfs alpine_rootfs -- /bin/sh

# 进入已运行的实例
revm attach ./alpine_rootfs
```

### docker-mode 模式
快速启动 podman 软件栈
```shell
revm docker-mode --data-storage ~/data.disk
```

docker-mode 的使用非常简单，一旦运行 docker-engine 跑起来后， 你就可以通过设置 `CONTAINER_HOST` 变量（podman cli 所使用）或者 `DOCKER_HOST`（docker cli 所使用的）到 `unix:///tmp/docker_api.sock` 来使用 docker/podman cli 命令。

```shell
# Docker cli 
export DOCKER_HOST=unix:///tmp/docker_api.sock
docker info

# Podman cli
export CONTAINER_HOST=unix:///tmp/docker_api.sock 
podman system info
```

# ⚙️ 高级用法

## 挂载镜像文件到 guest 中
```shell
# 自动挂载 data1.disk、data2.disk 到 guest 内的 /var/tmp/mnt/
revm rootfs-mode --rootfs alpine_rootfs \
  --data-disk ~/data1.disk \
  --data-disk ~/data2.disk \
  -- /bin/sh

# 日志打印  
INFO[2025-09-09T17:34:27+08:00] mount "/Users/danhexon/data1.disk" -> "/var/tmp/mnt/Users/danhexon/data1.disk"
INFO[2025-09-09T17:34:27+08:00] mount "/Users/danhexon/data2.disk" -> "/var/tmp/mnt/Users/danhexon/data2.disk"
```

## 挂载 Hots 的文件夹到 guest 中
```shell
# 将 Host 中的 /Users/danhexon 挂载到 guest 中的 /tmp/hostfs/danhexon
revm rootfs-mode --rootfs alpine_rootfs --mount /Users/danhexon:/tmp/hostfs/danhexon -- /bin/sh
```


## 继承 host 的代理设置
使用 `--system-proxy` 将代理设置传入 guest 中：
```shell
revm rootfs-mode --rootfs alpine_rootfs --system-proxy -- /bin/sh
```

# BUG 报告
https://github.com/ihexon/revm/issues

