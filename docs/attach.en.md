# revm attach

revm attach connects to a running session. It does not build a VM, start a VM, or change boot options.

Normal commands use the guest-control vsock endpoint. An interactive PTY uses the SSH compatibility path because guest-control currently provides streaming command execution rather than terminal allocation.

## Syntax

~~~text
revm attach --id <session-id> [--pty] [-- <command> [args...]]
~~~

Run a command:

~~~bash
revm attach --id dev -- sh -c 'uname -a'
revm attach --id dev -- podman ps
~~~

With no command and no --pty, attach runs /bin/sh:

~~~bash
revm attach --id dev
~~~

Open an interactive shell:

~~~bash
revm attach --id dev --pty
~~~

--pty does not take a guest command after it; it opens an interactive root SSH shell.

## Session lookup

attach uses --id to locate:

~~~text
~/.cache/revm/<session-id>/socks/vmctl.sock
~~~

The target session must still be running. A stopped session retains logs and resources but cannot be attached.

Start and attach:

~~~bash
revm run --id dev -- sh
revm attach --id dev --pty
~~~

Container session:

~~~bash
revm dockerd --id containers --podman-api /tmp/revm-containers.sock
revm attach --id containers -- sh -c 'podman ps'
~~~

## Control-plane relationship

attach only handles guest commands and terminals:

- guest-control carries command stdin, stdout, stderr, and exit status.
- --pty reads SSH metadata from the management API, then uses a gvisor tunnel or a direct tsi address.
- attach does not change port mappings; use ctl for that.
- attach does not stop the VM; run or dockerd owns VM shutdown.

## Logs and diagnostics

~~~bash
revm attach --id dev --log-level debug -- date
tail -f ~/.cache/revm/dev/logs/revm.log
~~~

When attach reports a missing session, verify that --id exactly matches the command that started it and check that the management socket exists. Guest command failures are returned with the guest exit error and stderr.

## Options

| Option | Description |
| --- | --- |
| --id | Required session name. |
| --pty | Open an interactive terminal through the SSH compatibility path. |
| --log-level | trace, debug, info, warn, error, fatal, or panic. |
