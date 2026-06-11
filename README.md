# revm

`revm` runs lightweight Linux microVM sessions from one CLI entrypoint.

It has four user-facing subcommands:

- `revm run`: boot a Linux rootfs and run a command.
- `revm dockerd`: boot the built-in container runtime and expose a Docker-compatible Podman API socket.
- `revm attach`: connect to an existing session.
- `revm ctl`: operate on an existing session, including port forwarding updates and rootfs import/export.

The host CLI is intentionally small: start a session, control a session, or connect external tools to the sockets that the session publishes.

## Quick Start

Run a command in the built-in Linux environment:

```bash
revm run --id shell -- sh -c 'uname -a && cat /etc/os-release'
```

Start a container runtime session:

```bash
revm dockerd --id dev --podman-api /tmp/revm-dev.sock
export DOCKER_HOST=unix:///tmp/revm-dev.sock
docker run --rm hello-world
```

Attach to an existing session:

```bash
revm attach --id dev --pty
```

Expose a guest port on the host:

```bash
revm ctl --id dev --list-port
revm ctl --id dev --port-export 127.0.0.1:8080:80
curl http://127.0.0.1:8080
revm ctl --id dev --port-unexport 127.0.0.1:8080
```

Export or import a session rootfs:

```bash
revm ctl --id dev --export-rootfs ./rootfs.tar.zst
revm ctl --id imported-dev --import-rootfs ./rootfs.tar.zst
```

## Sessions

Every command requires `--id`. The session ID is the stable name used for runtime state, sockets, logs, generated SSH keys, and later control operations.

By default, session state is stored under:

```text
~/.cache/revm/<session-id>
```

Important paths inside a session:

- `socks/vmctl.sock`: management API for the running VM.
- `socks/gvpctl.sock`: gvproxy control API used by port forwarding.
- `socks/podman-api.sock`: default Podman API proxy socket for `revm dockerd`.
- `logs/revm.log`: host-side command and VM lifecycle log.
- `ssh/ssh-key`: generated private key used for attach and internal control.
- `rootfs`: session root filesystem used by rootfs import/export.

Use `--manage-api`, `--podman-api`, `--ssh-key`, and `--log-to` when a stable external path is needed.

## Command Model

`revm run` creates a rootfs session and runs the command after `--` inside the guest.

```bash
revm run --id build \
  --mount "$PWD:/workspace" \
  --workdir /workspace \
  -- sh -c 'make test'
```

`revm dockerd` creates a long-running container session.

```bash
revm dockerd --id containers \
  --podman-api /tmp/revm-containers.sock \
  --mount "$PWD:/workspace"
```

`revm attach` and `revm ctl` never boot a new VM. They only talk to an existing session.

```bash
revm attach --id containers --pty
revm attach --id containers -- sh -c 'cat /etc/os-release'
revm ctl --id containers --list-port
revm ctl --id containers --port-export 8080:80
```

## Networking

`revm run` defaults to `--network gvisor`; `revm dockerd` always uses gvisor networking. Port export and unexport require gvisor because they are implemented through gvproxy.

Port specs are IPv4-only and TCP-only:

```text
--port-export [tcp:]<host-port>:<guest-port>
--port-export [tcp:]<host-ip>:<host-port>:<guest-port>
--port-unexport [tcp:]<host-port>
--port-unexport [tcp:]<host-ip>:<host-port>
```

If the host IP is omitted, `127.0.0.1` is used.

## Build

Build the release bundle from source:

```bash
go run ./scripts --build revm
```

The build reads `deps.lock`, downloads the pinned dependency archives from this repository's `deps-*` GitHub release, verifies SHA-256 checksums, embeds the guest agent helpers, links the host binary, and writes release output under `out/revm`.

Dependency archives are built separately by the manual `build-deps` workflow. Source inputs for that workflow are pinned in `deps/sources.lock`; the published asset release consumed by `revm` is pinned in `deps.lock`.

Linux release archives are built to run on both glibc and musl based distributions. The public entrypoint in `bin/` is a launcher script that starts the bundled `.real` executable through the bundled glibc dynamic linker and library set in `lib/`. Run `bin/revm` directly after extracting an archive; do not bypass the launcher by running `bin/revm.real`.

## Documentation

- [Documentation index](docs/README.md)
- [Run commands in a VM](docs/run.en.md)
- [Run container workloads](docs/dockerd.en.md)
- [Attach to sessions](docs/attach.en.md)
- [Control existing sessions](docs/ctl.en.md)
