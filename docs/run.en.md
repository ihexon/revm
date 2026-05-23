# revm run

`revm run` boots a Linux rootfs session and executes a command inside the guest. Use it for builds, tests, scripts, disposable debugging, and local tools that need a clean Linux runtime.

## Usage

```bash
revm run --id <session-id> [flags] -- <command> [args...]
```

`--id` is required. Everything after `--` is executed inside the guest.

```bash
revm run --id quick -- sh -c 'uname -a && cat /etc/os-release'
```

Open an interactive shell:

```bash
revm run --id shell -- sh
```

Mount the current project and run tests:

```bash
revm run --id build \
  --mount "$PWD:/workspace" \
  --workdir /workspace \
  -- sh -c 'make test'
```

## rootfs

When `--rootfs` is omitted, `revm run` uses the built-in rootfs.

Use a custom rootfs:

```bash
revm run --id ubuntu --rootfs ~/rootfs/ubuntu -- bash
```

A custom rootfs must provide an executable `/bin/sh`. Any other tools required by the command must also be present in that rootfs.

## Resources

```bash
revm run --id test \
  --cpus 4 \
  --memory 4096 \
  -- sh -c './test.sh'
```

- `--cpus`: number of vCPUs. If unset or less than 1, revm uses the host CPU count.
- `--memory`: memory in MB. If unset, revm uses host memory; the minimum is 512 MB.

## Files And Directories

Share directories with VirtIO-FS:

```bash
revm run --id dev \
  --mount "$PWD:/workspace" \
  --mount "$HOME/.cache/go-build:/go-cache,ro" \
  --workdir /workspace \
  -- sh
```

Mount format:

```text
--mount /host/path:/guest/path[,ro]
```

Attach ext4 raw disks with `--raw-disk`:

```bash
revm run --id disk \
  --raw-disk ~/.cache/revm/data.ext4,mnt=/data,version=v1 \
  -- sh -c 'df -h /data'
```

Disk format:

```text
--raw-disk <path>[,uuid=<uuid>][,version=<string>][,mnt=<guest-path>]
```

If the file does not exist, revm creates it. `version` is useful for rebuildable cache or data disks whose contents have a known schema.

## Environment And Proxy

Pass environment variables:

```bash
revm run --id env \
  --envs GOPROXY=https://proxy.golang.org,direct \
  --envs CI=true \
  -- sh -c 'env | sort'
```

Reuse the macOS system proxy:

```bash
revm run --id proxy --system-proxy -- sh -c 'curl -I https://example.com'
```

In gvisor network mode, system proxy endpoints that point at `127.0.0.1` are rewritten to a host address reachable from the guest.

## Network

`revm run` defaults to gvisor networking:

```bash
revm run --id net --network gvisor -- sh
```

Supported values:

- `gvisor`: gvisor-tap-vsock with NAT, DNS, port forwarding, and container-friendly networking.
- `tsi`: libkrun transparent socket interception. It is lighter, but does not support `revm ctl --port-export`.

## Expose Guest Ports

Port updates are performed by `revm ctl` against an existing session. They are not parsed by the `revm run` boot path.

Start a long-running service:

```bash
revm run --id web -- sh -c 'cd /tmp && python3 -m http.server 8000'
```

Expose it from another terminal:

```bash
revm ctl --id web --list-port
revm ctl --id web --port-export 127.0.0.1:8080:8000
curl http://127.0.0.1:8080
revm ctl --id web --port-unexport 127.0.0.1:8080
```

Port listing and port updates require gvisor networking. `--list-port` shows SSH, container-published ports, and manually exposed ports.

## Attach

`revm attach` can connect to an existing `run` session:

```bash
revm attach --id web --pty
revm attach --id web -- sh -c 'ps aux'
```

`revm attach` never creates a new VM. It fails when the session does not exist.

## Logs And Control Socket

Default log path:

```text
~/.cache/revm/<session-id>/logs/revm.log
```

Set log output explicitly:

```bash
revm run --id build --log-level debug --log-to /tmp/revm-build.log -- sh -c 'make test'
```

Export the management API socket:

```bash
revm run --id build --manage-api /tmp/revm-build-vmctl.sock -- sh
```

The management API is used by `revm ctl` to resolve attach metadata and the gvproxy endpoint.
