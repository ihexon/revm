# revm attach

`revm attach` connects to an existing session. It never starts a new VM and does not process boot options; it only reads the session management API, resolves SSH metadata, and enters the guest.

## Usage

```bash
revm attach --id <session-id> [--pty] [-- <command> [args...]]
```

Attach interactively:

```bash
revm attach --id dev --pty
```

Run a command:

```bash
revm attach --id dev -- sh -c 'uname -a'
```

When no command is provided and `--pty` is not set, `/bin/sh` is executed.

## Session

`--id` must reference a running session:

```bash
revm run --id dev -- sh
revm attach --id dev --pty
```

Or:

```bash
revm dockerd --id containers --podman-api /tmp/revm-containers.sock
revm attach --id containers -- sh -c 'podman ps'
```

## How It Works

`revm attach` reads the session management API:

```text
~/.cache/revm/<session-id>/socks/vmctl.sock
```

The management API returns the SSH key, guest address, and gvproxy tunnel metadata. `revm attach` then connects to the Dropbear SSH server inside the guest.

## Logs

Default log path:

```text
~/.cache/revm/<session-id>/logs/revm.log
```

Set log output:

```bash
revm attach --id dev --log-level debug --log-to /tmp/revm-attach.log -- sh -c 'date'
```

## Attach Versus ctl

`revm attach` connects to the guest and runs user commands.

`revm ctl` performs control-plane updates, such as port export and port unexport.
