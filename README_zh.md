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

---

## 文档

| 文档 | 说明 |
|------|------|
| [chroot 模式](docs/chroot-mode_zh.md) | macOS 上的 Linux chroot 替代方案——以近乎原生的性能运行任意 rootfs |
| [docker 模式](docs/docker-mode_zh.md) | 无需 Docker Desktop 的完整容器引擎——兼容 Podman/Docker CLI |
| [单文件分发](docs/single-binary_zh.md) | 自包含的 `revm-single` 分发，内嵌所有依赖 |
| [attach](docs/attach_zh.md) | 连接到运行中的 VM 实例 |
| [工作区与网络](docs/insider_zh.md) | 工作区结构、复用/清理，以及网络模式（gvisor / tsi） |
| [管理 API](docs/management-api.md) | 通过 Unix socket 访问的 VM 管理 API |

## 问题反馈

https://github.com/ihexon/revm/issues

## 许可证

Apache License 2.0 — 详见 [LICENSE](./LICENSE)。

> Some parts of this document were written using AI assistance because I was lazy.
