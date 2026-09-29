# revm 文档

[English index](README.en.md)

revm 是一个以 session 为单位管理 libkrun microVM 的小型 CLI。每个 session 由一个稳定的 --id 标识，主机负责 VM 生命周期，Alpine guest 负责命令和容器工作负载。

## 先看这里

- [快速运行命令](run.md)
- [运行 Podman 容器工作负载](dockerd.md)
- [连接已有 session](attach.md)
- [控制 gvisor 端口映射](ctl.md)

## 设计约束

- VM 始终启动程序内置的 Alpine rootfs。
- 不支持 --rootfs，也不再提供 rootfs 导入或导出命令。
- 自定义发行版或 rootfs 应作为 Podman workload 运行在 Alpine VM 内。
- 非交互命令通过 guest-control vsock 执行；SSH 只保留为 PTY 和内部兼容入口。
- VirtIO-FS 共享目录中的已有文件在 guest 中保留宿主机 UID、GID 和权限；revm 不预处理外部共享目录。
- run 和 dockerd 创建 VM；attach 和 ctl 只连接已有 VM。

## 命令关系

~~~text
run       -> 启动 Alpine，执行一次命令
dockerd   -> 启动 Alpine + Podman API，持续提供容器运行时
attach    -> 连接已有 session，执行命令或打开 PTY
ctl       -> 查询或修改 gvisor TCP 端口映射
~~~

## 公共约定

所有命令都需要 --id。默认 session 目录是：

~~~text
~/.cache/revm/<session-id>/
~~~

日志默认写入 session 目录中的 logs/revm.log。管理 socket、Podman socket 和 SSH key 都可以通过对应命令的路径选项导出到自定义位置。

网络模式有 gvisor 和 tsi 两种。gvisor 提供 gvisor-tap-vsock、NAT、DNS 和端口控制；tsi 使用 libkrun 的透明 socket interception，但不支持 ctl 的手动端口映射。

## 构建文档

- [依赖构建说明](../deps/README.md)
- [guest agent 内部说明](../cmd/guest-agent/README.md)

English versions:

- [run](run.en.md)
- [dockerd](dockerd.en.md)
- [attach](attach.en.md)
- [ctl](ctl.en.md)
