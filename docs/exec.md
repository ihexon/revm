# revm exec

[English](exec.en.md)

连接正在运行的 session，执行一条命令；不启动新 VM。不指定命令时执行 `/bin/sh`。

```sh
revm exec --id dev -- sh -c 'uname -a'
printf 'stream\n' | revm exec --id dev -- cat
revm exec --id dev --tty=always -- sh
revm exec --id dev --tty=always -- printf hello
```

普通命令和 PTY 均使用 guest-control v2，参数原样传递。stdin 按流传输；非 PTY 模式下 stdout、stderr 分开。guest 退出码被直接传递，guest 信号映射为 Linux 的 `128+signal`。

`--tty=auto` 默认只在 revm 位于前台，且 stdin/stdout 都是终端时分配 PTY。`--tty=always` 和 `never` 覆盖此判断。PTY 的输入 EOF 不自动发送 Ctrl-D。

exec 通过状态记录将 session ID 解析到本次实例的管理 socket。停止的 session 不可 exec；`revm ls` 和 `revm inspect ID` 可查看状态。

收到 INT、TERM、HUP、QUIT 时，客户端立即退出并断开连接；guest 立即杀死本次命令及其后代，不关闭整个 VM。终端模式保留本地 Ctrl-C 信号，在退出和暂停前恢复终端，resize 会传到 guest。

```sh
revm wait --ready --timeout 30s dev
revm logs --guest -f dev
revm stop dev
```

旧 guest 缺少 v2 能力时会明确报错，需要重启 session。端口管理使用 `ctl`，生命周期管理见 [session 命令](sessions.md)。运行 `revm exec --help` 查看全部选项。
