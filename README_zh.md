<p align="center">
  <img src="./icon.png" alt="revm logo" width="520" />
</p>
<h1 align="center"><b>revm</b></h1>
<p align="center">快速启动 Linux 虚拟机 / 容器的轻量工具</p>

[![build.yml](https://github.com/ihexon/revm/actions/workflows/build.yml/badge.svg)](https://github.com/ihexon/revm/actions/workflows/build.yml)

> [!WARNING]
> 该项目目前处于重度开发阶段

[README_EN](./README.md) | [README_ZH](./README_zh.md)

基于 [libkrun](https://github.com/containers/libkrun) 的轻量级 macOS Linux 虚拟机。提供两种独立模式：**chroot 模式**（在隔离
Linux Rootfs 环境中快速执行命令）和 **docker 模式**（在 Apple Silicon/Linux 上运行完整的 Podman 容器引擎）。

## 系统要求

macOS Sonoma 或更高版本

## 安装

```bash
wget https://github.com/ihexon/revm/releases/download/<TAG>/revm-Darwin-arm64.tar.zst

xattr -d com.apple.quarantine revm-Darwin-arm64.tar.zst

tar -xvf revm-Darwin-arm64.tar.zst
```

## 命令概览

`cmd/revm` 对外暴露三个用户命令：

| 命令 | 别名 | 作用 |
|------|------|------|
| `revm chroot` | `run` | 用自定义或内置 rootfs 启动 Linux microVM，并在其中执行命令 |
| `revm dockerd` | `start` | 启动内置容器虚拟机，并在宿主机暴露兼容 Podman 的 API socket |
| `revm attach` | — | 通过 SSH 重新连接已有会话，可进入交互 shell（`--pty`）或执行一次性命令 |

虚拟机内部实际运行的是 `cmd/guest-agent`：它负责挂载伪文件系统和共享磁盘、配置网络（`gvisor` 或
`tsi`）、启动 SSH 和可选的 Podman 服务、在 `chroot` 模式中执行用户命令，并把就绪状态回报给宿主机。

## 快速开始

```bash
# 在 rootfs 虚拟机里执行命令
revm chroot --id build --rootfs ~/ubuntu-rootfs -- bash -lc 'uname -a'

# 在另一终端重新进入同一个会话
revm attach --pty build

# 启动内置容器引擎
revm dockerd --id engine
export CONTAINER_HOST=unix:///tmp/engine/socks/podman-api.sock
podman run --rm alpine uname -a
```

## 关键参数

这些参数定义在 `cmd/revm/flags.go` 中，除特别说明外由 `chroot` / `dockerd` 共用。

| 参数 | 适用命令 | 说明 |
|------|----------|------|
| `--id` | 两者 | 会话名；工作目录默认是 `/tmp/<id>`，同一个 ID 不能并发启动两次 |
| `--cpus`, `--memory` | 两者 | 虚拟机 CPU 和内存配置 |
| `--mount` | 两者 | 通过 VirtIO-FS 将宿主目录共享到虚拟机 |
| `--raw-disk` | 两者 | 挂载 ext4 原始磁盘镜像；文件不存在时会自动创建 |
| `--network` | 两者 | 选择 `gvisor`（虚拟网卡、NAT、DNS、支持端口映射）或 `tsi`（透明套接字拦截） |
| `--system-proxy` | 两者 | 读取 macOS 系统代理并传入虚拟机 |
| `--manage-api` | 两者 | 自定义宿主机侧 VM 管理 API 的 Unix socket 路径 |
| `--ssh-key` | 两者 | 将生成的 SSH 私钥软链接到指定路径 |
| `--report-events` | 两者 | 将生命周期事件上报到 HTTP 端点 |
| `--log-level`, `--log-to` | 两者 | 控制宿主机侧日志级别和输出位置 |
| `--rootfs`, `--workdir`, `--envs` | `chroot` | 指定 rootfs，以及命令执行目录和环境变量 |
| `--container-disk`, `--podman-api` | `dockerd` | 控制容器持久化磁盘与导出的 Podman 兼容 socket 路径 |

---

## 文档

| 文档 | 说明 |
|------|------|
| [chroot 模式](docs/chroot-mode_zh.md) | macOS 上的 Linux chroot 替代方案——以近乎原生的性能运行任意 rootfs |
| [docker 模式](docs/docker-mode_zh.md) | 无需 Docker Desktop 的完整容器引擎——兼容 Podman/Docker CLI |
| [attach](docs/attach_zh.md) | 连接到运行中的 VM 实例 |
| [工作区与网络](docs/insider_zh.md) | 会话目录结构、复用/清理，以及网络模式（gvisor / tsi） |
| [管理 API](docs/management-api.md) | 通过 Unix socket 访问的 VM 管理 API |

## 问题反馈

https://github.com/ihexon/revm/issues

## 许可证

Apache License 2.0 — 详见 [LICENSE](./LICENSE)。

> Some parts of this document were written using AI assistance because I was lazy.
