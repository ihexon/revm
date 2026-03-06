# revm attach — connect to a running VM

Attach to a running VM instance from another terminal.

```bash
revm attach [--pty] <session-id> [-- <command> [args...]]
```

| Flag           | Description                                                                                                      | Default |
|----------------|------------------------------------------------------------------------------------------------------------------|---------|
| `--pty`        | Allocate a pseudo-terminal and launch an interactive shell; without this flag the command runs non-interactively | `false` |
| `--log-level`  | Log verbosity: `trace`, `debug`, `info`, `warn`, `error`, `fatal`, `panic`                                       | `info`  |

The `<session-id>` maps to the session directory `/tmp/<session-id>`.

```bash
# Interactive shell
revm attach --pty my-session

# Run a single command
revm attach my-session -- ps aux
```
