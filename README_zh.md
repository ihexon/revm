<h1 align="center"><b>revm</b></h1>
<p align="center">快速启动 Linux 虚拟机 / 容器的轻量工具</p>

[![build.yml](https://github.com/ihexon/revm/actions/workflows/build.yml/badge.svg)](https://github.com/ihexon/revm/actions/workflows/build.yml)

> [!WARNING]
> 该项目目前处于重度开发阶段

[README_EN](./README.md) | [README_ZH](./README_zh.md)

基于 [libkrun](https://github.com/containers/libkrun) 的轻量级 macOS Linux 虚拟机。提供两种独立模式：**chroot 模式**（在隔离
Linux Rootfs 环境中快速执行命令）和 **docker 模式**（在 Apple Silicon 上运行完整的 Podman 容器引擎）。

## 系统要求

macOS Sonoma 或更高版本

## 安装

```bash
wget https://github.com/ihexon/revm/releases/download/<TAG>/revm-Darwin-arm64.tar.zst

xattr -d com.apple.quarantine revm-Darwin-arm64.tar.zst

tar -xvf revm-Darwin-arm64.tar.zst
```

---

## chroot 模式 — macOS 上的 Linux chroot 替代方案

revm 的 chroot 模式通过启动一个真正的 Linux Kernel 来执行 Rootfs 内的二进制程序。由于底层使用 libkrun + Apple Hypervisor，
**启动时间通常在 1-2 秒以内**，近乎原生效率，隔离性更好，可直接在 macOS 上通过 `chroot` 模式进入任何 Rootfs。

### 典型使用场景

**快速运行任何 Rootfs**

```bash
# 用自己的 Ubuntu rootfs 做集成测试
revm chroot --rootfs ~/ubuntu-jammy -- bash -c 'apt-get install -y libssl-dev && make test'

# 只需快速启动一个 Linux Shell，直接使用内置 Alpine Rootfs
revm chroot -- sh -c 'uname -r'
```

**挂载宿主机源码目录进行编译**

```bash
revm chroot \
  --rootfs ~/ubuntu-rootfs \
  --mount /Users/me/myproject:/workspace \
  --workdir /workspace \
  bash -c 'make && ./run_tests.sh'
```

**保持 VM 存活，在需要的时候交互式 attach 到已经运行中的 Rootfs**

```bash
export WORKSPACE=/tmp/dev-env

# 终端 1：保持 VM 存活
revm chroot --workspace $WORKSPACE --rootfs ~/ubuntu-rootfs sleep 86400

# 终端 2：进入交互式 Shell
revm attach --pty $WORKSPACE

# 终端 3：执行一条命令
revm attach $WORKSPACE -- df -h
```

**挂载持久化数据盘**

```bash
# 第一次运行时自动创建 ext4 磁盘镜像，revm 会自动挂载到 /mnt/<UUID> 下
revm chroot --raw-disk ~/data.ext4 sh -c 'mount'

# 下次运行时复用同一块盘，数据持久保留
revm chroot --raw-disk ~/data.ext4 sh -c 'ls /mnt'
```

### 参数列表

```bash
revm chroot [flags] <command> [args...]
```

| 参数               | 说明                                                | 默认值                   |
|------------------|---------------------------------------------------|-----------------------|
| `--rootfs`       | rootfs 目录路径，须包含 `/bin/sh`；不指定则使用内置 Alpine         | 内置 Alpine             |
| `--cpus`         | 分配的 vCPU 核心数；不指定或小于 1 时自动取宿主机核心数                  | 宿主机核心数                |
| `--memory`       | VM 内存大小（MB）；最小 512 MB；不指定时自动取宿主机可用内存              | 宿主机可用内存               |
| `--workdir`      | 进入 VM 后执行命令前的工作目录                                 | `/`                   |
| `--mount`        | 通过 VirtIO-FS 挂载宿主机目录（格式：`/host:/guest[:ro]`，可重复）  | —                     |
| `--raw-disk`     | 挂载 ext4 裸盘镜像，不存在时自动创建 10 GB 镜像（可重复）               | —                     |
| `--envs`         | 传入环境变量（格式：`KEY=VALUE`，可重复）                        | —                     |
| `--network`      | 网络栈：`gvisor`（完整虚拟网卡）或 `tsi`（透明 socket 转发）         | `gvisor`              |
| `--system-proxy` | 读取 macOS 系统代理并以 `http_proxy`/`https_proxy` 注入到 VM | `false`               |
| `--workspace`    | 运行时状态目录（socket、SSH key、日志、磁盘）；不指定则临时目录            | `/tmp/.revm-<random>` |
| `--log-level`    | 日志级别：`trace`、`debug`、`info`、`warn`、`error`、`fatal`、`panic` | `info`          |
| `--report-url`   | 接收 VM 生命周期事件的 HTTP 端点（如 `unix:///var/run/events.sock`） | —               |

---

## docker 模式 — 你不需要安装 Docker Desktop 就可以迅速启动完整的 container 引擎

revm 内建完整的容器引擎，并通过 Unix socket 暴露给 podman-cli/docker-cli 调用，无需安装 Docker Desktop 或 Podman Desktop，即可快速启动完整的轻量容器软件栈。

### 快速开始

**启动容器引擎**

```bash
export WORKSPACE=~/revm_workspace
revm docker --workspace $WORKSPACE
```

启动后 Podman API socket 暴露在 `$WORKSPACE/socks/podman-api.sock`。

**用 podman 或 docker CLI 连接**

```bash
export CONTAINER_HOST=unix:///$WORKSPACE/socks/podman-api.sock

# 查看运行环境信息
podman info

# 运行容器（与 docker 命令完全兼容）
podman run --rm ubuntu:latest uname -r
podman run --rm -it alpine:edge sh
podman run --rm -p 8080:80 nginx
```

**用 docker CLI 连接**

```bash
export DOCKER_HOST=unix:///$WORKSPACE/socks/podman-api.sock
docker run --rm hello-world
```

### 端口映射

docker 模式下，容器的端口映射（`-p`）通过 gvproxy 自动转发到 macOS 本机：

```bash
podman run --rm -p 8888:80 nginx
# macOS 上直接访问
curl http://127.0.0.1:8888
```

### 挂载宿主机目录

```bash
podman run --rm -v /Users/me/data:/data ubuntu:latest ls /data
```

### 系统代理透传

在需要代理访问网络的环境下，加上 `--system-proxy` 自动读取 macOS 系统代理并注入到容器内：

```bash
revm docker --workspace $WORKSPACE --system-proxy

# 容器内 apt/curl 自动走代理
podman run --rm ubuntu:latest apt-get update
```

### 参数列表

```bash
revm docker [flags]
```

| 参数               | 说明                                                               | 默认值                   |
|------------------|------------------------------------------------------------------|-----------------------|
| `--cpus`         | 分配的 vCPU 核心数；不指定或小于 1 时自动取宿主机核心数                                 | 宿主机核心数                |
| `--memory`       | VM 内存大小（MB）；最小 512 MB；不指定时自动取宿主机可用内存                             | 宿主机可用内存               |
| `--mount`        | 通过 VirtIO-FS 挂载宿主机目录（格式：`/host:/guest[:ro]`，可重复）                 | —                     |
| `--raw-disk`     | 挂载 ext4 裸盘镜像，不存在时自动创建镜像（可重复）                                     | —                     |
| `--network`      | 网络栈：`gvisor`（完整虚拟网卡，支持端口映射）或 `tsi`（透明转发）                         | `gvisor`              |
| `--system-proxy` | 读取 macOS 系统代理并注入容器内，自动将 127.0.0.1 重写为 `host.containers.internal` | `false`               |
| `--workspace`    | 运行时状态目录，Podman API socket 在此目录下的 `socks/podman-api.sock`         | `/tmp/.revm-<random>` |
| `--log-level`    | 日志级别：`trace`、`debug`、`info`、`warn`、`error`、`fatal`、`panic`      | `info`                |
| `--report-url`   | 接收 VM 生命周期事件的 HTTP 端点（如 `unix:///var/run/events.sock`）           | —                     |

docker 模式与 chroot 模式共用大部分参数，可按需灵活配置。

---

## revm attach — 连接到运行中的 VM

在另一个终端连入正在运行的 VM 实例。

```bash
revm attach [--pty] <workspace> [-- <command> [args...]]
```

| 参数             | 说明                              | 默认值     |
|----------------|---------------------------------|---------|
| `--pty`        | 分配伪终端，启动交互式 Shell；不加则以非交互方式执行命令 | `false` |
| `--log-level`  | 日志级别：`trace`、`debug`、`info`、`warn`、`error`、`fatal`、`panic` | `info` |
| `--report-url` | 接收 VM 生命周期事件的 HTTP 端点（如 `unix:///var/run/events.sock`） | — |

```bash
# 交互式 Shell
revm attach --pty ~/revm_workspace

# 执行单条命令
revm attach ~/revm_workspace -- ps aux
```

---

## 工作区结构

`--workspace` 目录下的文件：

```
$WORKSPACE/
├── socks/
│   ├── podman-api.sock   # Podman API socket（docker 模式）
│   ├── gvpctl.sock       # gvproxy 控制 socket（gvisor 模式）
│   ├── vnet.sock         # 虚拟网络 socket（gvisor 模式）
│   ├── vmctl.sock        # VM 管理 API socket
│   └── ign.sock          # Ignition 配置服务 socket
├── ssh/
│   └── private.key       # 自动生成的 SSH 私钥
├── logs/
│   └── vm.log            # VM 内部日志
├── rootfs/               # 客户机根文件系统（chroot 模式）
└── raw-disk/             # 容器存储磁盘（docker 模式）
```

### 复用与清理

**复用**：下次启动时指定同一个 `--workspace`，VM 会直接加载上次的状态——容器镜像、卷数据、rootfs、SSH key 全部保留，无需重新配置或重新拉取镜像：

```bash
# 第一次启动
revm docker --workspace ~/revm_workspace

# 下次启动，镜像和数据原样保留
revm docker --workspace ~/revm_workspace
```

**临时环境**：不指定 `--workspace` 时，revm 自动使用 `/tmp/.revm-<random>` 临时目录，VM 退出后可安全删除，适合一次性任务或 CI 场景。

**清理**：删除整个 workspace 目录即可彻底重置，下次启动会创建全新环境：

```bash
rm -rf ~/revm_workspace
```

## 网络模式（ TSI/GVISOR 互斥）

docker 模式和 chroot 模式都支持 TSI 和 GVISOR 两种网络模式，这两种模式是互斥的

### gvisor（默认）

使用 [gvisor-tap-vsock](https://github.com/containers/gvisor-tap-vsock) 用户态网络栈。VM 固定 IP 为 `192.168.127.2`，网关为
`192.168.127.1`。容器内可通过 `host.containers.internal` 访问宿主机服务（如本机代理、本机 API 等）。

### tsi（透明 socket 拦截）

TSI（Transparent Socket Impersonation）是 libkrun 内置的网络模式，**不创建虚拟网卡**，guest 与 host 直接共享网络——两者可互相访问对方的 TCP/UDP 服务，无需特殊 IP 或端口转发规则。

与 gvisor 相比：**不支持 `-p` 端口映射**（无 gvproxy），也不提供 `host.containers.internal` 域名。在 TSI 模式下运行容器时，使用 `podman run --network=host` 可让容器直接复用 host 网络，端口在 macOS 上可直接访问，无需任何额外映射。

## 问题反馈

https://github.com/ihexon/revm/issues

## 许可证

Apache License 2.0 — 详见 [LICENSE](./LICENSE)。

> Some parts of this document were written using AI assistance because I was lazy.