# revm

`revm` 帮助你快速启动 Linux 虚拟机 / Container，快如闪电

---

## ✨ 特性

- ⚡ **轻量级**：一秒内进入 Linux shell，一秒拉起容器引擎（podman）
- 🧹 **干净**：不会更改你机器的任何配置
- 🐳 **容器兼容**：100% 兼容 Docker 命令行生态
- 📦 **灵活运行**：Rootfs模式和 Container 模式
- 💽 **磁盘挂载**：自动挂载外部虚拟磁盘文件（ext4/btrfs/xfs 多种格式）
- 📂 **目录挂载**：支持挂载宿主机文件到虚拟机中
- 🖥 **多终端支持**：随时 attach 到已运行的实例执行任何命令

---

## 🚀 快速开始

### 快速安装
```shell
$ wget https://github.com/ihexon/revm/releases/latest/download/revm.tar.zst
$ tar -xvf revm.tar.zst
$ ./out/bin/revm --help # help message
```

### 容器 模式
容器模式需要指定一块镜像文件作为 container 存储区域，通过 `--data-storage` 复用 & 生成镜像文件（ext4 格式）
```shell
revm docker-mode --data-storage ~/data.disk
```

通过设置 `CONTAINER_HOST` 变量（podman cli 所使用）或者 `DOCKER_HOST`（docker cli 所使用的）到 `unix:///tmp/docker_api.sock` 来使用 docker/podman cli 命令。

```shell
# Docker cli 
export DOCKER_HOST=unix:///tmp/docker_api.sock
docker info

# Podman cli
export CONTAINER_HOST=unix:///tmp/docker_api.sock 
podman system info
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

