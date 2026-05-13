# dockerd mode

`dockerd` mode starts a lightweight Linux container environment on your machine. You can keep using Docker CLI or Podman CLI, while containers run in an isolated Linux environment.

It is a good fit for teams that want familiar container workflows without relying on a heavy desktop container runtime.

## Who It Is For

- Developers who want Docker CLI compatibility with a lighter runtime.
- macOS or Linux users who need a stable local Linux container environment.
- Teams embedding container execution into internal developer tools, CI helpers, or local platforms.
- Engineering teams that want container state, project mounts, proxy settings, and logs controlled through the command line.

## Common Scenarios

### Scenario 1: Run Isolated Containers With Docker CLI

Start the runtime:

```bash
./dockerd --id dev --podman-api /tmp/dockerd-dev.sock
```

Use Docker CLI from another terminal:

```bash
export DOCKER_HOST=unix:///tmp/dockerd-dev.sock
docker run --rm hello-world
```

You keep the same command-line workflow while using an isolated Linux container environment.

### Scenario 2: Build Containers For A Local Project

Mount the project directory and build an image:

```bash
./dockerd --id app \
  --podman-api /tmp/dockerd-app.sock \
  --mount "$PWD:/workspace"
```

```bash
export DOCKER_HOST=unix:///tmp/dockerd-app.sock
docker build -t app /workspace
docker run --rm app
```

This works well for local service development, image builds, dependency checks, and team-wide development environments.

### Scenario 3: Keep Images And Container Data

Use a persistent disk if you do not want to pull images again or lose container data:

```bash
./dockerd --id dev --container-disk ~/.cache/dockerd-container.ext4
```

This keeps the container environment isolated while preserving long-lived state.

### Scenario 4: Test Web Service Ports

Container ports can be published to the host:

```bash
docker run --rm -p 8080:80 nginx
curl http://127.0.0.1:8080
```

This is useful for testing web services, APIs, frontend build outputs, and local integration flows.

### Scenario 5: Add Container Execution To Internal Tools

Wrap `dockerd` inside your own developer platform or automation tool:

```bash
./dockerd --id ci \
  --podman-api /tmp/dockerd-podman.sock \
  --mount "$PWD:/workspace" \
  --log-level info
```

Higher-level tools can connect to the socket and get a stable container execution backend.

## Quick Start

Start:

```bash
./dockerd --id dev --podman-api /tmp/dockerd-dev.sock
```

Use Podman CLI:

```bash
export CONTAINER_HOST=unix:///tmp/dockerd-dev.sock
podman run --rm alpine uname -a
```

Use Docker CLI:

```bash
export DOCKER_HOST=unix:///tmp/dockerd-dev.sock
docker ps
docker run --rm hello-world
```

## Core Capabilities

- Docker CLI and Podman CLI compatibility.
- Project directory mounts for local development and image builds.
- Container port publishing for service testing.
- Optional persistent container storage.
- Host proxy support for pulling dependencies and images.
- Logs and stable connection points for automation.

## Recommended Usage

For daily development, keep a fixed session:

```bash
./dockerd --id dev \
  --podman-api /tmp/dockerd-dev.sock \
  --mount "$PWD:/workspace"
```

Add a persistent disk when container state should survive across runs:

```bash
./dockerd --id dev \
  --podman-api /tmp/dockerd-dev.sock \
  --mount "$PWD:/workspace" \
  --container-disk ~/.cache/dockerd-container.ext4
```

For team tool integration, use a stable socket path:

```bash
./dockerd --id team \
  --podman-api /tmp/dockerd-podman.sock \
  --mount "$PWD:/workspace"
```
