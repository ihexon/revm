# revm

revm runs Linux commands in a small libkrun VM. It works on Apple Silicon macOS and on Linux amd64/arm64.

Each VM has a name. The name is used for its sockets, logs, keys, and disks, so you can come back to the same VM with attach or ctl.

The VM always boots the Alpine rootfs shipped with revm. If you need a different userspace, start the container mode and run it with Podman. The VM rootfs itself is fixed.

## Install

Download a release for your platform, unpack it, and run bin/revm. Linux bundles include the loader and libraries they need; keep the directory layout from the archive.

To build the application from a checkout:

~~~bash
go run ./scripts --build revm
~~~

libkrun, libkrunfw, and the Alpine rootfs are built by the GitHub Actions dependency workflow. The application build downloads the versions recorded in deps.lock; it does not rebuild those projects on the local machine.

## Run a command

~~~bash
revm run --id shell -- sh
revm run --id build \
  --mount "$PWD:/workspace" \
  --workdir /workspace \
  -- sh -c 'make test'
~~~

Everything after the double dash is run in the guest. The default network is gvisor. Use --network tsi when you want libkrun's transparent socket interception and do not need ctl port forwarding.

On Apple Silicon, `--gpu venus` adds a headless virtio-GPU device. The guest exposes `/dev/dri/card0` and `/dev/dri/renderD128`; revm does not open a macOS window or forward input events. GPU support is disabled unless requested explicitly:

~~~bash
revm run --id gpu --gpu venus -- ls -l /dev/dri
~~~

The built-in Alpine rootfs contains Mesa's virtio Vulkan driver, the Vulkan loader, and `vulkan-tools`. The current libkrun main Venus path still panics in its `magma-gpu` worker when `vulkaninfo` creates a device; the virtio-GPU device smoke test works, but Vulkan workloads remain blocked by that upstream issue. Venus depends on the libkrun and virglrenderer artifacts from the dependency workflow, so a dependency release must be rebuilt after changing those projects.

The rootfs already contains the guest agent and the tools used to bring up the network, mount filesystems, and run commands. It also includes Podman and the packages installed by the Alpine rootfs build. There is no custom rootfs option and no rootfs import/export command.

## Run containers

~~~bash
revm dockerd --id containers --podman-api "$PWD/podman.sock"
export DOCKER_HOST="unix://$PWD/podman.sock"
docker run --rm alpine uname -a
~~~

Podman clients can use the same socket through CONTAINER_HOST. The container storage disk lives in the session directory by default. Give --container-disk a path if the storage should survive removal of the session directory.

The host home directory is shared into container sessions at the same path. Add more shares with --mount.

## Attach to a running VM

~~~bash
revm attach --id shell
revm attach --id shell --pty
revm attach --id containers -- podman ps
~~~

A normal attach uses the guest-control vsock service. The PTY form uses the SSH compatibility service because it needs terminal allocation. SSH is not required for ordinary command execution.

## Forward a port

Port forwarding is available for sessions using gvisor:

~~~bash
revm ctl --id web --list-port
revm ctl --id web --port-export 127.0.0.1:8080:8000
revm ctl --id web --port-unexport 127.0.0.1:8080
~~~

Forward specifications are TCP and IPv4:

~~~text
[host-ip:]host-port:guest-port
~~~

If the host address is omitted, revm uses 127.0.0.1. The SSH port used by revm is reserved.

## Files and disks

The default session directory is:

~~~text
~/.cache/revm/<id>/
~~~

It contains the log, management sockets, generated SSH key, extracted Alpine rootfs, and any session-local disks. A normal exit leaves the directory in place.

Share a host directory with VirtIO-FS:

~~~bash
revm run --id files --mount "$PWD:/workspace" -- sh
~~~

Existing shared files retain the host UID, GID, and mode in the guest. revm does not pre-process external VirtIO-FS directories.

Attach a raw disk when a command needs persistent data:

~~~bash
revm run --id disk \
  --raw-disk "$HOME/.cache/revm/data.ext4,mnt=/data,version=v1" \
  -- sh -c 'df -h /data'
~~~

Images that do not exist are created. Changing a disk version recreates the image.

## Stopping

The first Ctrl-C asks libkrun to shut the guest down. The host keeps the management and network services alive until the guest exits. A second Ctrl-C, a lost launcher, or a failed host service uses the bounded force-stop path.

## Logs

Logs go to ~/.cache/revm/<id>/logs/revm.log by default.

~~~bash
revm run --id build --log-level debug --log-to /tmp/revm-build.log -- sh -c 'make test'
tail -f ~/.cache/revm/build/logs/revm.log
~~~

Use --manage-api, --podman-api, or --ssh-key when another program needs a socket or key at a known path.

## Development

Run the Go tests:

~~~bash
PKG_CONFIG_PATH="$(brew --prefix libarchive)/lib/pkgconfig:$(brew --prefix e2fsprogs)/lib/pkgconfig" \
DYLD_LIBRARY_PATH=/tmp/.deps/libkrun/lib \
go test ./...
~~~

Dependency builds are defined in .github/workflows/build-deps.yml. Their source revisions are in deps/sources.lock and the released archives are pinned in deps.lock.

## More documentation

- docs/run.md and docs/run.en.md
- docs/dockerd.md and docs/dockerd.en.md
- docs/attach.md and docs/attach.en.md
- docs/ctl.md and docs/ctl.en.md
- deps/README.md
- cmd/guest-agent/README.md
