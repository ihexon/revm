# revm podman

```sh
# Terminal 1
revm volume create data
revm podman --id containers --volume data --podman-api /tmp/revm-podman.sock

# Terminal 2
export CONTAINER_HOST=unix:///tmp/revm-podman.sock
podman info
podman run --rm -p 8080:80 docker.io/library/nginx
revm ports containers
```

Alternatively, use an external disk. Missing disks are created; existing disks are preserved:

```sh
revm podman --id containers --container-disk "$HOME/containers.ext4"
revm inspect containers  # In another terminal, find the Podman API address
```

Specify exactly one of `--volume` or `--container-disk`. Volumes survive session removal.
`--podman-api` creates a custom connection address and refuses to overwrite existing files.

```sh
revm exec --id containers -- podman ps
revm logs --guest containers
revm stop containers
revm rm containers
revm volume ls
revm volume rm data
```

Forward ordinary guest services with startup option `--port-export HOST_PORT:GUEST_PORT`;
use `podman -p` for containers. The VM runs in the foreground. Ctrl-C and `revm stop`
terminate it immediately; applications must flush their writes.
Use `revm podman --help` for CPU, memory, mounts and environment options.
