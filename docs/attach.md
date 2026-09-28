# revm attach

[English](attach.en.md)

revm attach 连接到一个正在运行的 session。它不构建、不启动新 VM，也不修改启动参数。

普通命令通过 guest-control vsock 执行。交互式 PTY 使用 SSH 兼容入口，因为 guest-control 当前提供的是流式命令执行而不是终端分配。

## 语法

~~~text
revm attach --id <session-id> [--pty] [-- <command> [args...]]
~~~

执行命令：

~~~bash
revm attach --id dev -- sh -c 'uname -a'
revm attach --id dev -- podman ps
~~~

不指定命令且不使用 --pty 时，默认执行 /bin/sh：

~~~bash
revm attach --id dev
~~~

打开交互式 shell：

~~~bash
revm attach --id dev --pty
~~~

--pty 不接受后续 guest 命令参数；它直接打开 root 用户的交互式 SSH shell。

## session 查找

attach 使用 --id 定位：

~~~text
~/.cache/revm/<session-id>/socks/vmctl.sock
~~~

目标 session 必须正在运行。停止后的 session 只有日志和资源文件，不可以 attach。

启动与连接示例：

~~~bash
revm run --id dev -- sh
revm attach --id dev --pty
~~~

容器 session：

~~~bash
revm dockerd --id containers --podman-api /tmp/revm-containers.sock
revm attach --id containers -- sh -c 'podman ps'
~~~

## 控制面关系

attach 只处理 guest 命令和终端：

- guest-control endpoint 负责普通命令的 stdin、stdout、stderr 和退出码。
- --pty 通过管理 API 读取 SSH metadata，再通过 gvisor tunnel 或 tsi 直连 guest SSH。
- attach 不负责端口映射；端口映射使用 ctl。
- attach 不负责关闭 VM；停止由 run 或 dockerd 的宿主进程处理。

## 日志和诊断

~~~bash
revm attach --id dev --log-level debug --log-to /tmp/revm-attach.log -- date
tail -f ~/.cache/revm/dev/logs/revm.log
~~~

如果 attach 报 session 不存在，先确认 --id 与启动命令完全一致，并检查管理 socket 是否存在。如果 guest 命令失败，attach 会转发 guest 的退出错误和 stderr。

## 选项

| 选项 | 说明 |
| --- | --- |
| --id | 必填 session 名称。 |
| --pty | 通过 SSH 兼容入口打开交互终端。 |
| --log-level | trace、debug、info、warn、error、fatal 或 panic。 |
| --log-to | 自定义 host 日志文件。 |
