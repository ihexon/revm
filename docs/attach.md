# revm attach

[English](./attach.en.md)

`revm attach` 连接到已有 session。它不会启动新 VM，也不会处理启动参数；它只读取 session 的管理 API，获取 SSH 连接信息，然后进入 guest。

## 基本用法

```bash
revm attach --id <session-id> [--pty] [-- <command> [args...]]
```

交互式连接：

```bash
revm attach --id dev --pty
```

执行命令：

```bash
revm attach --id dev -- sh -c 'uname -a'
```

不指定命令且不使用 `--pty` 时，默认执行 `/bin/sh`。

## Session

`--id` 必须指向一个正在运行的 session：

```bash
revm run --id dev -- sh
revm attach --id dev --pty
```

或：

```bash
revm dockerd --id containers --podman-api /tmp/revm-containers.sock
revm attach --id containers -- sh -c 'podman ps'
```

## 工作方式

`revm attach` 会访问 session 的管理 API：

```text
~/.cache/revm/<session-id>/socks/vmctl.sock
```

管理 API 返回 SSH key、guest 地址和 gvproxy tunnel 信息。随后 `revm attach` 使用这些信息连接 guest 内的 Dropbear SSH server。

## 日志

默认日志路径：

```text
~/.cache/revm/<session-id>/logs/revm.log
```

指定日志：

```bash
revm attach --id dev --log-level debug --log-to /tmp/revm-attach.log -- sh -c 'date'
```

## 与 ctl 的区别

`revm attach` 负责连接 guest 并执行用户命令。

`revm ctl` 负责控制面更新，例如端口暴露和取消暴露。
