# revm run

revm run creates a session, boots the Alpine rootfs packaged with the release, and executes one command inside the guest. It is intended for builds, tests, scripts, disposable Linux tools, and local tasks that need isolation.

## Syntax

~~~text
revm run --id <session-id> [options] -- <command> [args...]
~~~

--id is required. Arguments after -- form the guest command. Use sh -c explicitly when a command needs shell syntax.

~~~bash
revm run --id shell -- sh -c 'uname -a; cat /etc/os-release'
revm run --id build --mount "$PWD:/workspace" --workdir /workspace -- sh -c 'make test'
~~~

When the command exits, revm asks the guest to shut down and exits. Run a long-lived process in the guest, or use revm dockerd, when a session must stay available.

## Rootfs policy

The VM rootfs always comes from the Alpine archive embedded in the release. It includes:

- the Alpine userspace and apk package manager
- guest-agent
- the dropbear SSH compatibility service
- Podman, containers configuration, and container networking tools
- tools required for networking, mounts, logging, and lifecycle control

run does not accept --rootfs and does not replace, import, or export the VM rootfs. To use another distribution or userspace, start revm dockerd and run the image as a Podman workload. The workload then remains separate from the VM control plane.

## Resources and networking

~~~bash
revm run --id test --cpus 4 --memory 4096 -- sh -c './test.sh'
revm run --id net --network gvisor -- sh
revm run --id light --network tsi -- sh
~~~

Options:

| Option | Description |
| --- | --- |
| --cpus | vCPU count. Host CPU count when unset or less than 1; maximum 32. |
| --memory | Memory in MB. Host total memory when unset; minimum 512. |
| --network | gvisor or tsi, default gvisor. |
| --workdir | Guest command working directory, default /. |
| --envs KEY=VALUE | Environment for the guest command; repeatable. |
| --system-proxy | Read the macOS HTTP/HTTPS proxy and pass it to the guest. In gvisor mode, loopback proxy addresses are rewritten. |

gvisor uses gvisor-tap-vsock for DNS, NAT, TCP/UDP, and port forwarding. tsi uses libkrun transparent socket interception; it has a smaller host networking path but does not support manual ctl port mappings.

## VirtIO-FS directories

Use --mount to share a host directory:

~~~text
--mount /host/path:/guest/path[,ro]
~~~

~~~bash
revm run --id files \
  --mount "$PWD:/workspace" \
  --mount "$HOME/.cache:/host-cache,ro" \
  --workdir /workspace \
  -- sh
~~~

Existing shared inodes retain the host UID, GID, and mode in the guest. revm does not pre-process external shared directories.

Omit ro when the guest must write. Mount sources are resolved to absolute paths and must be under the user's home directory or /tmp.

## Raw disks

Use --raw-disk to attach a raw/ext4 disk in the guest. Missing images are created automatically.

~~~text
--raw-disk <path>[,uuid=<uuid>][,version=<string>][,mnt=<guest-path>][,readonly=true][,directio=true][,sync=none|relaxed|full]
~~~

~~~bash
revm run --id disk \
  --raw-disk "$HOME/.cache/revm/data.ext4,version=v1,mnt=/data" \
  -- sh -c 'df -h /data'
~~~

Details:

- mnt must be an absolute guest path. When omitted, the disk's default mount target is used.
- version is stored and checked through user.vm.rawdisk.version; a mismatch recreates the image.
- readonly and directio accept true or false.
- sync accepts none, relaxed, or full.
- uuid selects the filesystem UUID when a new image is created; an existing image is not rewritten.

## Logs, sockets, and SSH keys

The default session directory is:

~~~text
~/.cache/revm/<session-id>/
~~~

Logs always go to the session directory:

~~~text
~/.cache/revm/<session-id>/logs/revm.log  host lifecycle and control plane
~/.cache/revm/<session-id>/logs/vm.log    guest-agent and compatibility services
~~~

~~~bash
revm run --id build --log-level debug -- sh -c 'make test'
revm run --id build --manage-api /tmp/revm-build-vmctl.sock -- sh
revm run --id build --ssh-key /tmp/revm-build-ssh-key -- sh
~~~

--manage-api and --ssh-key create symlinks to files inside the session. The management API is used by attach and ctl. Normal commands use guest-control over vsock; SSH remains mainly for --pty and compatibility.

## Stop behavior

The first SIGINT or SIGTERM requests the native libkrun shutdown. A second signal, launcher loss, or host-service failure enters the bounded force-stop path, which waits at most three seconds. A normal command exit returns the guest command result; failure to stop within the bound returns an error.

## Complete option list

~~~text
--id
--cpus
--memory
--envs
--raw-disk
--mount
--system-proxy
--workdir
--network
--manage-api
--ssh-key
--report-events
--log-level
~~~

Run revm run --help for the complete help text from the current binary.
