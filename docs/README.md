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

所有托管数据统一放在 `~/.revm`，可用 `REVM_HOME` 指定其他绝对路径。日志和结果位于 `state/sessions/<id>/runs/<instance>/`，运行时文件在 `run/`，私有 rootfs 在 `cache/instances/`，持久卷在 `data/volumes/`。`logs` 读取宿主日志，`logs --guest` 读取 guest 日志；`cache prune` 清理已停止实例的临时资源。XDG 环境变量不再分散这些目录。

## 构建说明

- [依赖构建](../deps/README.md)
- [guest agent](../cmd/guest-agent/README.md)

## 设计提案

- [资源生命周期设计](resource-lifecycle-design.md)：临时资源、运行记录与持久数据的边界、清理规则和迁移方案，待实现。
- [信号与终端行为设计](signal-terminal-design.md)：执行会话、TTY、信号路由、有界取消和退出结果，待实现。
