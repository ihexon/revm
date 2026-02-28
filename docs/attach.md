# revm attach — connect to a running VM

Attach to a running VM instance from another terminal.

```bash
revm attach [--pty] <workspace> [-- <command> [args...]]
```

| Flag           | Description                                                                                                      | Default |
|----------------|------------------------------------------------------------------------------------------------------------------|---------|
| `--pty`        | Allocate a pseudo-terminal and launch an interactive shell; without this flag the command runs non-interactively | `false` |
| `--log-level`  | Log verbosity: `trace`, `debug`, `info`, `warn`, `error`, `fatal`, `panic`                                       | `info`  |
| `--report-url` | HTTP endpoint to receive VM lifecycle events (e.g. `unix:///var/run/events.sock`)                                | —       |

```bash
# Interactive shell
revm attach --pty ~/revm_workspace

# Run a single command
revm attach ~/revm_workspace -- ps aux
```
