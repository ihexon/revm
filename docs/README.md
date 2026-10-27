# revm 文档

[English](README.en.md)

revm 在 libkrun VM 中执行 Linux 命令。所有命令和 PTY 共用 guest-control 流式协议。每次启动使用全新的 Alpine rootfs；需要保留的数据通过显式磁盘或目录挂载提供。

- [run：运行命令](run.md)
- [podman：容器服务](podman.md)
- [exec：在已有 VM 中运行新命令](exec.md)
- [ports：端口查询](ports.md)
- [session 管理](sessions.md)
- [资源所有权](resources.md)
- [退出状态](exit-status.md)
- [执行和信号协议](execution-protocol.md)

`run`、`podman` 省略 `--id` 时临时运行，成功后清理记录，失败时保留日志和结果；指定 `--id NAME` 时保留日志和结果。`exec`、`ports` 需要正在运行的 session 名称。

持久卷独立管理：`volume create NAME` 创建，`podman --volume NAME` 使用，`volume rm NAME` 删除。session 不隐式创建或重新关联卷。`--container-disk PATH` 可直接使用外部磁盘。

日志和结果位于 `${XDG_STATE_HOME:-~/.local/state}/revm/sessions/<id>/runs/<instance>/`。`logs` 读取宿主日志，`logs --guest` 读取 guest 日志。私有 rootfs 使用 XDG_CACHE_HOME，持久卷使用 XDG_DATA_HOME；`cache prune` 清理已停止实例的临时资源。

## 构建说明

- [依赖构建](../deps/README.md)
- [guest agent](../cmd/guest-agent/README.md)
