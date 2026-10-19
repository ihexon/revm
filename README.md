# revm

revm runs Linux commands in a libkrun VM on Apple Silicon macOS and Linux amd64/arm64. Commands use the built-in Alpine environment; Podman provides other container userspaces.

```sh
printf 'hello\n' | revm run --id pipe -- cat
revm run --rm --id build --mount "$PWD:/workspace" --workdir /workspace -- make test
revm run --id shell -- sh
```

Arguments after `--` reach the guest unchanged. Stdin, stdout and stderr stream immediately. Guest exit codes propagate to the shell. A terminal is allocated automatically when stdin and stdout are terminals and revm is in the foreground; `--tty=always|never` overrides this. `--pty` aliases `--tty=always` for run and attach.

## Install

Download the release archive and its `.sha256` file for your platform. Verify the archive before unpacking:

```sh
# macOS
shasum -a 256 -c revm-darwin-arm64.tar.zst.sha256
mkdir revm-release
tar -xf revm-darwin-arm64.tar.zst -C revm-release
./revm-release/bin/revm --version

# Linux: use revm-linux-x86_64 or revm-linux-aarch64
sha256sum -c revm-linux-x86_64.tar.zst.sha256
```

Keep the entire extracted bundle together. Its executable uses the sibling `lib` directory; Linux also uses the bundled loader. Add the bundle's `bin` directory to `PATH`. To upgrade, unpack into a new directory and switch `PATH` after existing sessions stop. Session state and volumes live outside the bundle.

```sh
export PATH="$PWD/revm-release/bin:$PATH"
# Bash requires bash-completion; zsh requires compinit.
source <(revm completion bash)
# or: source <(revm completion zsh)
# fish: revm completion fish > ~/.config/fish/completions/revm.fish
man ./revm-release/share/man/man1/revm.1
```

Releases include completion scripts, the manual, documentation and archive checksums. Linux needs accessible KVM; macOS uses Hypervisor.framework.

## Sessions

```sh
revm ls --json
revm inspect shell
revm attach --id shell -- sh -c 'uname -a'
revm attach --id shell --tty=always -- sh
revm wait --ready --timeout 30s shell
revm logs --guest -f shell
revm stop shell
revm wait shell                    # returns this instance's recorded exit status
revm rm shell                      # stopped sessions only; volumes remain
```

One session name has at most one active instance. Each start gets a new instance ID, a private writable rootfs, separate logs and a result record. A crash with no result is reported as `unknown`. `run --rm` removes session records and logs after success or failure.

The first INT, TERM, HUP or QUIT cancels the command with a shared shutdown budget (`--stop-timeout`, default 10s). A second signal accelerates cleanup. Attach cancellation affects its own command and descendants. The native VMM is a child whose lifetime follows a control pipe from its owner; losing the CLI also terminates the VMM.

## Containers

```sh
# Terminal 1
revm podman --id containers --podman-api /tmp/revm-containers.sock
# Terminal 2
export CONTAINER_HOST=unix:///tmp/revm-containers.sock
podman run --rm alpine uname -a
```

The default `podman-containers` volume persists independently of the session. Stop, restart and session removal preserve it; starting the same name reconnects it. `--container-disk PATH` uses a user-owned external disk. Podman sessions share the host home at the same path; add project shares with `--mount`.

```sh
revm volume ls --json
revm volume rm podman-containers   # explicit deletion; refuses an active volume
revm cache prune                  # only inactive resources with known ownership
```

## Files and storage

Runtime sockets and keys use a short private runtime directory. Rootfs copies use `${XDG_CACHE_HOME:-~/.cache}/revm/instances`; logs and results use `${XDG_STATE_HOME:-~/.local/state}/revm/sessions`; persistent volumes use `${XDG_DATA_HOME:-~/.local/share}/revm/volumes`. Roots cannot overlap. See [resource ownership](docs/resources.md).

```sh
revm run --id files --mount "$PWD:/workspace" -- sh
revm run --id disk --raw-disk "$HOME/data.ext4,mnt=/data,version=v1" -- df -h /data
```

Bind mounts keep host UID, GID and permissions. External disks remain user-owned, including disks created on first use. Existing disks are inspected; mismatched versions return an error without deleting data. Changes to the guest system rootfs are discarded after each instance.

Old cache layouts are not imported or used. Existing external disks can be selected explicitly with `--container-disk`.

## Networking

`run` defaults to gvisor with DNS, NAT and port forwarding. `--network tsi` uses libkrun transparent socket interception. Podman always uses gvisor.

```sh
revm ctl --id web --list-port --json
revm ctl --id web --port-export 127.0.0.1:8080:8000
revm ctl --id web --port-unexport 127.0.0.1:8080
```

Manual forwards support TCP/IPv4 and default to `127.0.0.1`. The internal SSH port is reserved. `--manage-api`, `--podman-api` and `--ssh-key` expose convenient symlinks; unrelated existing paths are never overwritten.

## Development

```sh
go run ./scripts --build revm
# Unit checks after dependencies are prepared by the build:
./scripts/test.sh
# Real VM checks on a host with virtualization support:
python3 tests/smoke.py
python3 tests/sessions.py
python3 tests/volumes.py
python3 tests/terminal.py
```

The build downloads dependencies pinned by `deps.lock`, compiles the guest agent, signs the macOS executable, and creates a relocatable bundle and checksum. `go run ./scripts --lint` also runs lint. The dependency workflow builds libkrun, libkrunfw and Alpine from `deps/sources.lock`.

See [session commands](docs/sessions.md), [exit status](docs/exit-status.md), [execution and signals](docs/execution-protocol.md), and the [command reference](docs/README.md).
