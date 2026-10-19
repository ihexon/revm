# revm podman

revm podman starts a long-lived Alpine VM, launches the Podman API service in the guest, and exposes a Unix socket on the host. The host Podman CLI uses that socket as its container endpoint.

revm podman always uses gvisor networking, so container port publishing and revm ctl port control are available.


## Usage

~~~text
revm podman --volume data --id <session-id> [options]
~~~

~~~bash
# Create independent storage, then start the VM.
revm volume create data
revm podman --volume data --id dev --podman-api /tmp/revm-dev.sock
~~~

Connect the host Podman client from another terminal:

~~~bash
export CONTAINER_HOST=unix:///tmp/revm-dev.sock
podman run --rm alpine uname -a
~~~

The service stays alive until a stop signal or host-service failure. Stopping revm podman removes the guest runtime from the client.

## API socket

--podman-api is the host Unix socket path. When omitted:

~~~text
$XDG_RUNTIME_DIR/revm/<instance>/socks/podman-api.sock
~~~

This socket forwards the native Podman byte stream over a private vsock channel to the guest service. Clients do not need guest IP, vsock, or SSH details. Transport uses `mdlayher/vsock`, `libp2p/go-yamux/v5`, and `inetaf/tcpproxy`. Yamux supplies stream flow control and half-close for `exec -i`, attach, builds, logs, and file transfers.

Podman listens only on `/run/revm/podman/podman.sock` in the guest, with directory mode `0700` and socket mode `0600`; there is no TCP management port. Both the Podman channel and guest-control accept only host CID 2, rejecting guest loopback and ordinary containers using host networking. Host control sockets reside inside the `0700` session directory; `--podman-api` creates a symlink to the API socket there.

Ordinary workloads must not mount the management socket. A trusted Supervisor can explicitly mount the guest `/run/revm/podman` directory; mounting the directory survives socket replacement when the service restarts. This grants management of the whole VM container engine. Privileged containers and containers with access to the guest root filesystem are outside this isolation boundary.

Export the management API separately:

~~~bash
revm podman --volume data --id dev \
  --podman-api /tmp/revm-dev.sock \
  --manage-api /tmp/revm-dev-vmctl.sock
~~~

## Project directories and home

Share a project with --mount:

~~~bash
revm podman --volume data --id app \
  --podman-api /tmp/revm-app.sock \
  --mount "$PWD:/workspace"
~~~

revm podman only shares explicit `--mount` paths; the host home directory is not mounted automatically. Existing shared inodes retain the host UID, GID, and mode in the guest; revm does not pre-process external shared directories.

Mount format:

~~~text
--mount /host/path:/guest/path[,ro]
~~~

A workload rootfs comes from the Podman image. revm --rootfs cannot replace the VM rootfs.

## Container storage

Container storage is mounted at /var/lib/containers from an ext4 raw disk.

Each launch requires `--volume NAME` or `--container-disk PATH`, never both.
Create a named volume with `revm volume create NAME`. Its disk lives at:

~~~text
${XDG_DATA_HOME:-~/.local/share}/revm/volumes/<volume-id>/disk.ext4
~~~

Use a persistent path:

~~~bash
revm podman --id dev \
  --podman-api /tmp/revm-dev.sock \
  --container-disk "$HOME/.cache/revm/container-storage.ext4,version=containers-v1"
~~~

Format:

~~~text
--container-disk <path>[,version=<string>]
~~~

Missing external disks are created and remain user-owned. Existing disks are inspected; mismatched versions return an error without replacing data. Stopping or removing a session preserves its managed volume. Every launch explicitly selects storage; session names do not reconnect disks.
Omit `--id` to remove session records at exit. Use `volume rm NAME` for explicit deletion; active volumes cannot be deleted. Legacy layouts are not imported; select an existing disk explicitly with `--container-disk`.

## Port publishing

Use the Podman -p option:

~~~bash
export CONTAINER_HOST=unix:///tmp/revm-dev.sock
podman run --rm -d --name revm-web -p 127.0.0.1:8080:80 nginx
curl http://127.0.0.1:8080
podman stop revm-web
~~~

The guest agent configures the Podman machine marker so container start and stop calls the gvisor forwarder's expose/unexpose operations.

Expose a non-container guest service with ctl:

~~~bash
revm ctl --id dev --list-port
revm ctl --id dev --port-export 127.0.0.1:8081:8081
revm ctl --id dev --port-unexport 127.0.0.1:8081
~~~

ctl port formats support TCP and IPv4 only. A run session using tsi does not support these operations.

## Resources, environment, and proxy

~~~bash
revm podman --volume data --id dev \
  --cpus 4 \
  --memory 4096 \
  --envs CI=true \
  --system-proxy \
  --podman-api /tmp/revm-dev.sock
~~~

| Option | Description |
| --- | --- |
| --cpus | vCPU count, min(host CPU count, 4) by default, maximum 32. |
| --memory | Memory in MB, one quarter of available memory bounded to 512–2048 MB by default, minimum 512. |
| --envs KEY=VALUE | Environment passed to the Podman service and container runtime; repeatable. |
| --system-proxy | Pass the macOS system proxy into the guest. |
| --raw-disk | Add extra raw/ext4 disks; repeatable. |
| --container-disk | Set the Podman container storage disk. |
| --mount | Add VirtIO-FS shares; repeatable. |
| --podman-api | Custom Podman API socket. |
| --manage-api | Custom VM management socket. |
| --report-events | HTTP endpoint for lifecycle events. |
| --log-level | Set the log level. Logs always go to the session directory. |

revm podman has no --network option; it always uses gvisor.

## Exec and ctl

~~~bash
revm exec --id dev --tty=always
revm exec --id dev -- sh -c 'podman ps'
revm ctl --id dev --list-port
~~~

exec uses guest-control v2 for both ordinary commands and PTYs. ctl only changes port mappings; it does not execute guest commands.

## Stop behavior

The first stop signal requests guest shutdown over guest-control and waits for
`--stop-timeout`. A second signal or timeout forces cleanup, bounded to three seconds. The container storage disk is synced during a normal stop.

See [resource ownership](resources.md) and [session management](sessions.md). Runtime paths may use the private `/tmp/revm-<uid>` fallback; logs and results are stored under XDG_STATE_HOME.
