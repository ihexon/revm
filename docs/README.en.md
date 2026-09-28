# revm documentation

[中文索引](README.md)

revm is a small CLI that manages libkrun microVM sessions. Every session has one stable --id. The host owns the VM lifecycle, while the Alpine guest runs commands and container workloads.

## Start here

- [Run a command](run.en.md)
- [Run Podman workloads](dockerd.en.md)
- [Attach to an existing session](attach.en.md)
- [Control gvisor port mappings](ctl.en.md)

## Design boundaries

- Every VM boots the Alpine rootfs packaged with the revm release.
- --rootfs and rootfs import/export commands are removed.
- A different distribution or userspace should run as a Podman workload inside the Alpine VM.
- Non-interactive commands use the guest-control vsock endpoint. SSH remains only as a PTY and compatibility entrypoint.
- VirtIO-FS shares appear as root:root in the guest. On macOS, user.containers.override_stat stores the guest-visible UID, GID, and mode.
- run and dockerd create VMs; attach and ctl only connect to existing sessions.

## Command relationships

~~~text
run       -> boot Alpine and execute one command
dockerd   -> boot Alpine with the Podman API and keep the container runtime alive
attach    -> connect to a session, execute a command, or open a PTY
ctl       -> inspect or change gvisor TCP port mappings
~~~

## Common conventions

Every command requires --id. The default session directory is:

~~~text
~/.cache/revm/<session-id>/
~~~

Logs go to logs/revm.log in the session directory by default. The management socket, Podman socket, and SSH key can be exported to custom paths with the command flags.

The two network modes are gvisor and tsi. gvisor provides gvisor-tap-vsock, NAT, DNS, and port control. tsi uses libkrun transparent socket interception and does not support manual ctl port mappings.

## Build documentation

- [Dependency builds](../deps/README.md)
- [Guest agent internals](../cmd/guest-agent/README.md)

Chinese versions:

- [run](run.md)
- [dockerd](dockerd.md)
- [attach](attach.md)
- [ctl](ctl.md)
