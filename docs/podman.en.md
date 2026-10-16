# revm podman

revm podman starts a long-lived Alpine VM, launches the Podman API service in the guest, and exposes a Unix socket on the host. The host Podman CLI uses that socket as its container endpoint.

revm podman always uses gvisor networking, so container port publishing and revm ctl port control are available.


## Usage

~~~text
revm podman --id <session-id> [options]
~~~

~~~bash
# Terminal 1: start the VM and Podman service.
revm podman --id dev --podman-api /tmp/revm-dev.sock
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
~/.cache/revm/<session-id>/socks/podman-api.sock
~~~

This socket forwards the native Podman byte stream over a private vsock channel to the guest service. Clients do not need guest IP, vsock, or SSH details. Transport uses `mdlayher/vsock`, `libp2p/go-yamux/v5`, and `inetaf/tcpproxy`. Yamux supplies stream flow control and half-close for `exec -i`, attach, builds, logs, and file transfers.

Podman listens only on `/run/revm/podman/podman.sock` in the guest, with directory mode `0700` and socket mode `0600`; there is no TCP management port. Both the Podman channel and guest-control accept only host CID 2, rejecting guest loopback and ordinary containers using host networking. Host control sockets reside inside the `0700` session directory; `--podman-api` creates a symlink to the API socket there.

Ordinary workloads must not mount the management socket. A trusted Supervisor can explicitly mount the guest `/run/revm/podman` directory; mounting the directory survives socket replacement when the service restarts. This grants management of the whole VM container engine. Privileged containers and containers with access to the guest root filesystem are outside this isolation boundary.

Export the management API separately:

~~~bash
revm podman --id dev \
  --podman-api /tmp/revm-dev.sock \
  --manage-api /tmp/revm-dev-vmctl.sock
~~~

## Project directories and home

Share a project with --mount:

~~~bash
revm podman --id app \
  --podman-api /tmp/revm-app.sock \
  --mount "$PWD:/workspace"
~~~

revm podman also mounts the host home directory at the same path in the guest, which lets workloads use project files, credentials, and caches. Existing shared inodes retain the host UID, GID, and mode in the guest; revm does not pre-process external shared directories.

Mount format:

~~~text
--mount /host/path:/guest/path[,ro]
~~~

A workload rootfs comes from the Podman image. revm --rootfs cannot replace the VM rootfs.

## Container storage

Container storage is mounted at /var/lib/containers from an ext4 raw disk.

Without --container-disk, the disk is stored in the session workspace:

~~~text
~/.cache/revm/<session-id>/raw-disk/container-storage.ext4
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

Missing disks are created. An existing disk with no version xattr or a different version is recreated; recreation deletes its old images and containers. Keep the same version and file when data must survive.

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
revm podman --id dev \
  --cpus 4 \
  --memory 4096 \
  --envs CI=true \
  --system-proxy \
  --podman-api /tmp/revm-dev.sock
~~~

| Option | Description |
| --- | --- |
| --cpus | vCPU count, host CPU count by default, maximum 32. |
| --memory | Memory in MB, host total by default, minimum 512. |
| --envs KEY=VALUE | Environment passed to the Podman service and container runtime; repeatable. |
| --system-proxy | Pass the macOS system proxy into the guest. |
| --raw-disk | Add extra raw/ext4 disks; repeatable. |
| --container-disk | Set the Podman container storage disk. |
| --mount | Add VirtIO-FS shares; repeatable. |
| --podman-api | Custom Podman API socket. |
| --manage-api | Custom VM management socket. |
| --ssh-key | Symlink the compatibility SSH key to a custom path. |
| --report-events | HTTP endpoint for lifecycle events. |
| --log-level | Set the log level. Logs always go to the session directory. |

revm podman has no --network option; it always uses gvisor.

## Attach and ctl

~~~bash
revm attach --id dev --pty
revm attach --id dev -- sh -c 'podman ps'
revm ctl --id dev --list-port
~~~

attach uses guest-control for normal commands and SSH for --pty compatibility. ctl only changes port mappings; it does not execute guest commands.

## Stop behavior

The first SIGINT or SIGTERM requests native libkrun shutdown. A second signal or host-service failure triggers the bounded force-stop path, which waits at most three seconds. The container storage disk is synced during a normal stop.
