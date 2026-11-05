# revm exec

[中文](exec.md)

Execute a command in a running session without starting another VM. The default command is `/bin/sh`.

```sh
revm exec --id dev -- sh -c 'uname -a'
printf 'stream\n' | revm exec --id dev -- cat
revm exec --id dev --tty=always -- sh
revm exec --id dev --tty=always -- printf hello
```

Commands and PTYs both use guest-control v3 and preserve argv. Input streams
immediately; stdout and stderr stay separate without a PTY. Guest exit codes
propagate, with signals mapped to 128 plus the Linux signal number.

`--tty=auto` requires revm in the foreground with terminal stdin and stdout.
`always` and `never` override this. PTY input EOF never
injects Ctrl-D. Ctrl-C, Ctrl-Z and Ctrl-\ pass through to the guest terminal and
foreground job. Terminal state is restored on exit and host suspend; resize is forwarded.

Session records resolve the current instance's management socket. A stopped
session cannot execute commands; inspect it with `revm ls` or `revm inspect ID`.
INT, TERM, HUP and QUIT sent to the host process cancel only this execution and
its descendants, preserving the VM. The client exits immediately; disconnect
kills the execution subtree. Ctrl-C before the PTY is ready or without a PTY
still cancels locally.

```sh
revm wait --ready --timeout 30s dev
revm logs --guest -f dev
revm stop dev
```

Older guests without v3 return a capability error and must be restarted. Use
`revm ports SESSION` for ports and [session commands](sessions.md) for lifecycle management.
Run `revm exec --help` for all options.
