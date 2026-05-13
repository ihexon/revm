# revm

`revm` 是一个复合项目，主要作为 `chroot` 和 `dockerd` 的共享代码库与运行时基础

- `chroot`: 用于运行隔离的 Linux 命令环境。
- `dockerd`: 用于运行隔离的 Linux 容器环境，并兼容 Docker CLI / Podman CLI。

每个入口命令尽可能保持 KISS 原则。

## Guides

- [chroot mode](docs/chroot.md): 使用隔离的 Linux 环境运行命令、构建、测试和脚本。
- [dockerd mode](docs/dockerd.md): 使用 Docker CLI 或 Podman CLI 运行隔离的容器环境。
