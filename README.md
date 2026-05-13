# revm

revm runs lightweight Linux VMs with libkrun. It provides two command line tools:

- `chroot`: run a command inside a Linux rootfs.
- `dockerd`: start a VM with a Podman-compatible container engine.

This repository is a composite project:

- host CLIs in `cmd/chroot` and `cmd/dockerd`;
- a guest init/agent in `cmd/guest-agent`;
- Go bindings and orchestration code around libkrun in `pkg/`;
- build and packaging logic in `scripts/`;
- embedded runtime assets in `pkg/static_resources` and `cmd/guest-agent/pkg/service`.

## Run chroot

Use the built-in Alpine rootfs:

```bash
./chroot --id quick -- sh -c 'uname -a'
```

Use your own rootfs:

```bash
./chroot --id ubuntu --rootfs ~/ubuntu-rootfs -- bash
```

Mount a host directory:

```bash
./chroot --id build \
  --mount "$PWD:/work" \
  --workdir /work \
  -- make test
```

Mount it read-only:

```bash
./chroot --id inspect --mount "$PWD:/src,ro" -- ls /src
```

Attach a persistent raw disk:

```bash
./chroot --id data --raw-disk ~/.cache/revm-data.ext4 -- sh
```

## Run dockerd

Start the container engine:

```bash
./dockerd --id dev
```

In another terminal, point `podman` or `docker` at the socket:

```bash
export CONTAINER_HOST=unix://$HOME/.cache/revm/dev/socks/podman-api.sock
podman run --rm alpine uname -a
```

Docker CLI works too:

```bash
export DOCKER_HOST=unix://$HOME/.cache/revm/dev/socks/podman-api.sock
docker run --rm hello-world
```

Publish ports as usual:

```bash
podman run --rm -p 8080:80 nginx
curl http://127.0.0.1:8080
```

Use a persistent container storage disk:

```bash
./dockerd --id dev --container-disk ~/.cache/revm-container.ext4
```

## Common Flags

- `--id NAME`: session name. Runtime files live under `~/.cache/revm/NAME`.
- `--cpus N`: number of vCPUs.
- `--memory MB`: VM memory in MiB.
- `--mount /host:/guest[,ro]`: share a host directory with the VM.
- `--raw-disk PATH[,mnt=/guest/path]`: attach or create an ext4 raw disk.
- `--system-proxy`: pass the system HTTP proxy into the guest.
- `--log-level LEVEL`: `trace`, `debug`, `info`, `warn`, or `error`.

## Help

```bash
./chroot --help
./dockerd --help
```
