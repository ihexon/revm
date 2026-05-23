# revm dockerd

`revm dockerd` starts the built-in container runtime. It runs Podman inside the guest and exposes a Docker-compatible API socket on the host, so Docker CLI and Podman CLI can connect to it.

## Usage

```bash
revm dockerd --id <session-id> [flags]
```

Start a container session:

```bash
revm dockerd --id dev --podman-api /tmp/revm-dev.sock
```

Use Docker CLI from another terminal:

```bash
export DOCKER_HOST=unix:///tmp/revm-dev.sock
docker run --rm hello-world
```

Use Podman CLI:

```bash
export CONTAINER_HOST=unix:///tmp/revm-dev.sock
podman run --rm alpine uname -a
```

## API Socket

`--podman-api` sets the Unix socket exposed on the host:

```bash
revm dockerd --id team --podman-api /tmp/revm-team.sock
```

When omitted, the default path is inside the session directory:

```text
~/.cache/revm/<session-id>/socks/podman-api.sock
```

This socket is served by a host-side proxy that forwards to the Podman API inside the guest. Higher-level tools only need this socket; they do not need to know VM internals.

## Project Directories

Mount a project directory:

```bash
revm dockerd --id app \
  --podman-api /tmp/revm-app.sock \
  --mount "$PWD:/workspace"
```

Build an image:

```bash
export DOCKER_HOST=unix:///tmp/revm-app.sock
docker build -t app /workspace
docker run --rm app
```

Mount format:

```text
--mount /host/path:/guest/path[,ro]
```

## Container Storage

When `--container-disk` is omitted, revm uses the default container storage disk inside the session.

Use a persistent storage disk:

```bash
revm dockerd --id dev \
  --podman-api /tmp/revm-dev.sock \
  --container-disk ~/.cache/revm/container-storage.ext4
```

Format:

```text
--container-disk <path>[,version=<string>]
```

If the file does not exist, revm creates it. If the stored version is missing or does not match `version`, revm recreates the disk. This makes container storage manageable as a rebuildable cache.

## Port Publishing

Container port publishing continues to use Docker or Podman CLI:

```bash
export DOCKER_HOST=unix:///tmp/revm-dev.sock
docker run --rm -p 8080:80 nginx
curl http://127.0.0.1:8080
```

The guest agent configures the Podman machine marker so container start/stop calls gvproxy's expose/unexpose API.

To manually expose any service port from the guest, use `revm ctl`:

```bash
revm ctl --id dev --list-port
revm ctl --id dev --port-export 127.0.0.1:8081:8081
revm ctl --id dev --port-unexport 127.0.0.1:8081
```

`--list-port` shows SSH, container-published ports, and manually exposed ports.

## Resources, Proxy, And Logs

Set resources:

```bash
revm dockerd --id dev \
  --cpus 4 \
  --memory 4096 \
  --podman-api /tmp/revm-dev.sock
```

Reuse the macOS system proxy:

```bash
revm dockerd --id dev --system-proxy --podman-api /tmp/revm-dev.sock
```

Set log output:

```bash
revm dockerd --id dev \
  --log-level debug \
  --log-to /tmp/revm-dockerd.log \
  --podman-api /tmp/revm-dev.sock
```

Default log path:

```text
~/.cache/revm/<session-id>/logs/revm.log
```

## Attach And Control

Connect to a running container session:

```bash
revm attach --id dev --pty
revm attach --id dev -- sh -c 'podman ps'
```

Export the management API socket:

```bash
revm dockerd --id dev \
  --manage-api /tmp/revm-dev-vmctl.sock \
  --podman-api /tmp/revm-dev.sock
```

`revm attach` uses the management API to resolve SSH metadata and connect to the guest. `revm ctl` uses the management API to resolve the gvproxy endpoint and perform control-plane updates.
