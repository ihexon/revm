# revm attach — 连接到运行中的 VM

在另一个终端连入正在运行的 VM 实例。

```bash
revm attach [--pty] <session-id> [-- <command> [args...]]
```

| 参数             | 说明                              | 默认值     |
|----------------|---------------------------------|---------|
| `--pty`        | 分配伪终端，启动交互式 Shell；不加则以非交互方式执行命令 | `false` |
| `--log-level`  | 日志级别：`trace`、`debug`、`info`、`warn`、`error`、`fatal`、`panic` | `info` |

`<session-id>` 映射到会话目录 `/tmp/<session-id>`。

```bash
# 交互式 Shell
revm attach --pty my-session

# 执行单条命令
revm attach my-session -- ps aux
```
