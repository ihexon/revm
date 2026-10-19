# revm run

revm run creates a session, boots the Alpine rootfs packaged with the release, and executes one command inside the guest. It is intended for builds, tests, scripts, disposable Linux tools, and local tasks that need isolation.

## Syntax

~~~text
revm run [--id <session-id>] [options] -- <command> [args...]
~~~

Omit `--id` for a disposable run; specify it to retain logs and results. Arguments after -- form the guest command. Use sh -c explicitly when a command needs shell syntax.

~~~bash
revm run --id shell -- sh -c 'uname -a; cat /etc/os-release'
revm run --id build --mount "$PWD:/workspace" --workdir /workspace -- sh -c 'make test'
~~~

When the command exits, revm asks the guest to shut down and exits. Run a long-lived process in the guest, or use revm podman, when a session must stay available.

## Rootfs policy

The VM rootfs always comes from the Alpine archive embedded in the release. It includes:

- the Alpine userspace and apk package manager
- guest-agent
- Podman, containers configuration, and container networking tools
- tools required for networking, mounts, logging, and lifecycle control

run does not accept --rootfs and does not replace, import, or export the VM rootfs. To use another distribution or userspace, start revm podman and run the image as a Podman workload. The workload then remains separate from the VM control plane.

## Resources and networking

~~~bash
revm run --id test --cpus 4 --memory 4096 -- sh -c './test.sh'
revm run --id net --network gvisor -- sh
revm run --id light --network tsi -- sh
~~~

Options:

| Option | Description |
| --- | --- |
| --cpus | vCPU count. Defaults to min(host CPU count, 4); maximum 32. |
| --memory | Memory in MB. Defaults to one quarter of available memory, bounded to 512–2048 MB; minimum 512. |
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
- version is stored and checked through user.vm.rawdisk.version; a mismatch returns an error and preserves the image.
- readonly and directio accept true or false.
- sync accepts none, relaxed, or full.
- uuid selects the filesystem UUID when a new image is created; an existing image is not rewritten.

## Terminals, streams and results

Input streams immediately. Without a PTY, stdout and stderr remain separate.
Guest exit codes propagate. `--tty=auto` requires foreground terminal stdin and
terminal stdout; `always` and `never` override this.
PTY input EOF never injects Ctrl-D.

Each run has a private rootfs and temporary sockets. Logs and results live
under `${XDG_STATE_HOME:-~/.local/state}/revm/sessions/<id>/runs/<instance>`.
Use `revm logs ID`, `revm logs --guest -f ID`, and `revm inspect ID`.
Omitting `--id` removes records after success or failure, preserving persistent data.
See [resource ownership](resources.md) and [session commands](sessions.md).

INT, TERM, HUP and QUIT retain the first cancellation cause. `--stop-timeout`
defaults to 10s, shared by command cleanup and VM shutdown. A second signal
accelerates cleanup, followed by at most 3s for forced shutdown. The VMM child
exits on loss of its owner's control pipe.

Run `revm run --help` for all options.
