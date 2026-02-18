# revm

[![build.yml](https://github.com/ihexon/revm/actions/workflows/build.yml/badge.svg)](https://github.com/ihexon/revm/actions/workflows/build.yml)

> [!WARNING]
> 该项目目前处于重度开发阶段

基于 [libkrun](https://github.com/containers/libkrun) 的轻量级 macOS Linux 沙箱与容器启动器。可在 1 秒内启动完整的 Linux 环境或 Podman 容器引擎。

## 特性

- **亚秒级启动** — 通过 libkrun 轻量级虚拟机几乎瞬间启动 Linux Shell
- **内置 Podman 引擎** — 使用标准 `podman` CLI 运行容器，无需 Docker Desktop
- **零配置** — 自带 Alpine rootfs 和内核，开箱即用
- **多终端** — 通过 vsock SSH 将额外的 Shell 连接到运行中的虚拟机
- **目录共享** — 通过 VirtIO-FS 将宿主机目录挂载到客户机
- **裸磁盘挂载** — 挂载 ext4 磁盘镜像，客户机内自动挂载
- **持久化工作区** — 可选择跨运行保留 rootfs 和数据
- **自定义 rootfs** — 可使用自己的 Linux rootfs（Ubuntu、Debian 等）
- **灵活网络** — 可选 gvisor 用户态网络栈或 TSI（透明套接字拦截）
- **系统代理透传** — 将宿主机 HTTP/HTTPS 代理设置转发到客户机

## 系统要求

macOS Sonoma 或更高版本（Apple Silicon）

## 安装

```bash
# 下载最新版本
wget https://github.com/ihexon/revm/releases/download/<TAG>/revm-Darwin-arm64.tar.zst

# 移除 macOS 隔离属性
xattr -d com.apple.quarantine revm-Darwin-arm64.tar.zst

# 解压
tar -xvf revm-Darwin-arm64.tar.zst
```

## 快速开始

### 运行临时 Shell

不指定 `--workspace` 时，一切都是临时的（类似 `docker run --rm`）。

```bash
revm run sh             # 启动临时 Shell
revm run -- uname -a    # 运行单条命令
```

### 启动虚拟机实例并连接到客户机 Shell

使用长时间运行的命令（如 `sleep`）保持虚拟机存活，然后在另一个终端使用 `attach` 打开 Shell 或在客户机内执行命令。

```shell
export WORKSPACE=/tmp/my_work
revm run --workspace $WORKSPACE -- sleep 10000

# 交互式 Shell
revm attach --pty $WORKSPACE
# 非交互式命令
revm attach $WORKSPACE -- echo hello
```

### 启动容器引擎

Podman API 套接字暴露在 `$WORKSPACE/socks/podman-api.sock`。通过设置 `CONTAINER_HOST` 或 `DOCKER_HOST` 环境变量将 `docker`/`podman` CLI 指向它。

```shell
export WORKSPACE=/tmp/my_workspace
revm docker --log-level info --workspace $WORKSPACE

# 在另一个终端
export WORKSPACE=/tmp/my_workspace
export CONTAINER_HOST=unix:///$WORKSPACE/socks/podman-api.sock
podman info
podman run --rm -dit alpine:edge sh
```

## 命令

### `revm run` — 在 Linux 虚拟机中运行命令

使用内置（或自定义）rootfs 启动轻量级虚拟机并执行指定命令。

```bash
revm run [flags] <command> [args...]
```

| 参数             | 说明                                | 默认值                   |
|------------------|-----------------------------------|-----------------------|
| `--cpus`         | CPU 核心数                           | `1`                   |
| `--memory`       | 内存大小（MB）                          | `512`                 |
| `--workspace`    | 持久化状态的工作区目录                       | `/tmp/.revm-<random>` |
| `--rootfs`       | 自定义 rootfs 目录路径                   | 内置 Alpine             |
| `--workdir`      | 客户机内的工作目录                         | `/`                   |
| `--raw-disk`     | 挂载裸磁盘镜像（可重复）                      | —                     |
| `--mount`        | 挂载宿主机目录（可重复，格式：`host:guest[:ro]`） | —                     |
| `--envs`         | 设置环境变量（可重复，格式：`KEY=value`）        | —                     |
| `--network`      | 网络栈：`gvisor/tsi`                  | `gvisor`              |
| `--system-proxy` | 将宿主机 HTTP/HTTPS 代理转发到客户机          | `false`               |

**示例：**

```bash
revm run --workspace /tmp/my_workspace --rootfs ./my-ubuntu-rootfs bash
```

### `revm container` — 启动 Podman 容器引擎

启动带有内置 Podman 服务的虚拟机，并通过宿主机上的 Unix 套接字暴露 API。默认使用所有可用的 CPU 核心和内存。

```bash
revm container [flags]
```

| 参数             | 说明                        | 默认值                |
|------------------|----------------------------|-----------------------|
| `--cpus`         | CPU 核心数                  | 所有核心              |
| `--memory`, `-m` | 内存大小（MB）              | 所有可用内存          |
| `--workspace`    | 工作区目录                  | `/tmp/.revm-<random>` |
| `--raw-disk`     | 挂载裸磁盘镜像（可重复）     | —                     |
| `--mount`        | 挂载宿主机目录（可重复）     | —                     |
| `--network`      | 网络栈：`GVISOR` 或 `TSI`  | `GVISOR`              |
| `--system-proxy` | 将宿主机代理转发到容器引擎   | `false`               |

**示例：**

```shell
export WORKSPACE=/tmp/my_workspace
revm docker --log-level info --workspace $WORKSPACE

# 在另一个终端
export WORKSPACE=/tmp/my_workspace
export CONTAINER_HOST=unix:///$WORKSPACE/socks/podman-api.sock
podman info
podman run --rm -dit alpine:edge sh
```

### `revm attach` — 连接到运行中的虚拟机

通过 SSH 打开到运行中虚拟机实例的额外终端会话。适用于多终端工作流程。

```bash
revm attach [flags] <workspace-path> [-- command]
```

| 参数             | 说明                          | 默认值  |
|------------------|------------------------------|---------|
| `--pty`, `--tty` | 分配伪终端（交互式 Shell）     | `false` |

**示例：**

```bash
# 打开交互式 Shell
revm attach --pty ~/my_space

# 非交互式运行命令
revm attach ~/my_space -- ls -la /
```

### 全局参数

| 参数          | 说明                                                                      | 默认值 |
|---------------|--------------------------------------------------------------------------|--------|
| `--log-level` | 日志级别：`trace`、`debug`、`info`、`warn`、`error`、`fatal`、`panic`      | `warn` |

## 挂载宿主机目录到客户机

使用 `--mount` 参数通过 VirtIO-FS 将宿主机目录挂载到客户机。

**格式：** `host_path:guest_path[:ro]`

| 部分         | 必填 | 说明                                    |
|--------------|------|-----------------------------------------|
| `host_path`  | 是   | 宿主机上的绝对路径                       |
| `guest_path` | 否   | 客户机内的挂载点（默认与宿主机路径相同）  |
| `ro`         | 否   | 以只读方式挂载（默认为读写）              |

```bash
# 读写挂载
revm run --mount /Users/me/projects:/mnt/projects sh

# 只读挂载
revm run --mount /Users/me/data:/mnt/data:ro sh

# 多个挂载
revm run --mount /path/a:/mnt/a --mount /path/b:/mnt/b sh

# 省略客户机路径 — 挂载到客户机内相同路径
revm run --mount /Users/me/projects sh
```

## 挂载裸磁盘到客户机

使用 `--raw-disk` 参数挂载 ext4 磁盘镜像。每个磁盘在客户机内自动挂载到 `/mnt/<UUID>`，挂载选项为 `rw,discard`。

- 如果磁盘文件**不存在**，将自动创建并格式化一个 10 GB 的 ext4 镜像。
- 可通过重复 `--raw-disk` 参数挂载多个磁盘。

```bash
# 挂载单个磁盘（不存在时自动创建）
revm run --raw-disk ~/mydisk.ext4 sh

# 在客户机内
$ mount
/dev/vda on /mnt/1f803bc6-db09-48b7-96af-e027fd616afe type ext4 (rw,relatime,discard,data=ordered)

# 挂载多个磁盘
revm run --raw-disk ~/disk1.ext4 --raw-disk ~/disk2.ext4 sh
```

## 工作区

默认情况下客户机文件系统是临时的（存储在 `/tmp` 下）。要跨运行持久化数据，请传入一个固定的 `--workspace` 目录 — 所有 rootfs 变更和 SSH 密钥都将保存在此处。

```bash
$ revm run --workspace ~/my_space -- sh -c 'echo hello > /hello.txt'
$ revm run --workspace ~/my_space -- sh -c 'cat /hello.txt'
hello
```

工作区目录包含：

- `rootfs/` — 客户机根文件系统
- `ssh/` — 自动生成的 SSH 密钥对，用于宿主机与客户机通信
- `socks/` — Unix 套接字（ignition、vmctl、podman API、网络）
- `logs/` — 虚拟机日志文件
- `raw-disk/` — 容器存储磁盘（容器模式）

## 网络

### GVISOR（默认）

revm 使用 [gvisor-tap-vsock](https://github.com/containers/gvisor-tap-vsock) 作为用户态网络栈。客户机始终获得 IP `192.168.127.2`。可以在客户机内通过域名 `host.containers.internal` 访问宿主机上的 TCP 服务。

## 问题反馈

https://github.com/ihexon/revm/issues

## 许可证

Apache License 2.0 — 详见 [LICENSE](./LICENSE)。
