# revm documentation

[中文](README.md)

revm runs Linux commands in libkrun VMs. Every command and PTY uses the same streaming guest-control protocol. Each VM starts with a fresh Alpine rootfs; persistent data requires explicit disks or directory mounts.

- [run: execute a command](run.en.md)
- [podman: container service](podman.en.md)
- [exec: start a command in an existing VM](exec.en.md)
- [ports: port mappings](ports.en.md)
- [Session management](sessions.md)
- [Resource ownership](resources.md)
- [Exit status](exit-status.md)
- [Execution and signals](execution-protocol.md)

Omit `--id` to discard successful run records; failures retain logs and results. Use `--id NAME` to retain every run. `exec` and `ports` require a running session name.

Volumes are independent: `volume create NAME` creates storage, `podman --volume NAME` uses it, and `volume rm NAME` deletes it. Sessions do not implicitly create or reconnect volumes. `--container-disk PATH` selects an external disk.

All managed data lives under `~/.revm`, or the absolute path selected by `REVM_HOME`. Logs and results use `state/sessions/<id>/runs/<instance>/`, runtime files use `run/`, private rootfs copies use `cache/instances/`, and volumes use `data/volumes/`. Use `logs` for host logs and `logs --guest` for guest logs. `cache prune` removes inactive temporary resources. XDG variables no longer split these directories.

## Build documentation

- [Dependency builds](../deps/README.md)
- [Guest agent internals](../cmd/guest-agent/README.md)
