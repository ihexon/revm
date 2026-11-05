# revm exec

[English](exec.en.md)

连接正在运行的 session，执行一条命令；不启动新 VM。不指定命令时执行 `/bin/sh`。

```sh
revm exec --id dev -- sh -c 'uname -a'
printf 'stream\n' | revm exec --id dev -- cat
revm exec --id dev --tty=always -- sh
revm exec --id dev --tty=always -- printf hello
```

普通命令和 PTY 均使用 guest-control v3，参数原样传递。stdin 按流传输；非 PTY 模式下 stdout、stderr 分开。guest 退出码被直接传递，guest 信号映射为 Linux 的 `128+signal`。

`--tty=auto` 默认只在 revm 位于前台，且 stdin/stdout 都是终端时分配 PTY。`--tty=always` 和 `never` 覆盖此判断。PTY 的输入 EOF 不自动发送 Ctrl-D。

exec 通过状态记录将 session ID 解析到本次实例的管理 socket。停止的 session 不可 exec；`revm ls` 和 `revm inspect ID` 可查看状态。

PTY 模式下，Ctrl-C、Ctrl-Z、Ctrl-\ 作为输入交给 guest 终端和前台任务处理；客户端不会因此直接退出或暂停。在退出和宿主暂停前恢复本地终端，resize 会传到 guest。

宿主进程收到 INT、TERM、HUP、QUIT（例如通过 `kill`，或非 PTY 模式下按 Ctrl-C）时，客户端立即退出并断开连接；guest 立即杀死本次命令及其后代，不关闭整个 VM。PTY 就绪前的 Ctrl-C 仍取消启动。

```sh
revm wait --ready --timeout 30s dev
revm logs --guest -f dev
revm stop dev
```

旧 guest 缺少 v3 能力时会明确报错，需要重启 session。端口查询使用 `revm ports SESSION`，生命周期管理见 [session 命令](sessions.md)。运行 `revm exec --help` 查看全部选项。
