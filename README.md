# revm

revm runs Linux commands in a libkrun VM on Apple Silicon macOS and Linux amd64/arm64. Commands use the built-in Alpine environment; Podman provides other container userspaces.

```sh
printf 'hello\n' | revm run -- cat
revm run --mount "$PWD:/workspace" --workdir /workspace -- make test
revm run --id shell -- sh
```

Arguments after `--` reach the guest unchanged. Stdin, stdout and stderr stream immediately. Guest exit codes propagate to the shell. A terminal is allocated automatically when stdin and stdout are terminals and revm is in the foreground; `--tty=always|never` overrides this.

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

Run `revm COMMAND --help` for required arguments and copyable examples. Shell completion includes existing session and volume names.
After upgrading, reload or regenerate your shell completion script using the commands above.

Releases include completion scripts, the manual, documentation and archive checksums. Linux needs accessible KVM; macOS uses Hypervisor.framework.

## Sessions

```sh
revm ls --json
revm inspect shell
revm exec --id shell -- sh -c 'uname -a'
revm exec --id shell --tty=always -- sh
revm wait --ready --timeout 30s shell
revm logs --guest --tail 50 -f shell
revm stop shell
revm wait shell                    # returns this instance's recorded exit status
revm rm shell                      # stopped sessions only; volumes remain
```

`ls` shows the mode, start time, duration, exit code and command. `inspect` includes the log directory and active management/Podman connection URLs.

One session name has at most one active instance. Each start gets a new instance ID, a private writable rootfs, separate logs and a result record. A crash with no result is reported as `unknown`. Omitting `--id` creates a disposable run: records and logs are removed only after success; failures, interruptions and crashes retain them until `revm rm`. Specifying `--id` retains them.

The `revm run` / `podman` process is the VM: libkrun runs directly inside it. INT, TERM, HUP, QUIT and `revm stop` end that process immediately, without a shutdown grace period. Cancellation can discard buffered guest writes and pending output. Normal command completion drains output before exiting. revm does not shut down the guest or sync its filesystems; applications own flushing writes. Exec cancellation disconnects only its command; the guest kills that command's entire subtree.

A detached cleanup helper waits for the VM process to die, then restores the terminal if necessary and removes temporary resources. It retains failed-run logs and all persistent volumes. Cleanup can briefly continue after the shell receives the command status; `revm wait SESSION` waits for it. `cache prune` can recover leftovers if the helper itself fails.

## Containers

```sh
# Terminal 1
revm volume create data
revm podman --volume data --id containers --podman-api /tmp/revm-containers.sock
# Terminal 2
export CONTAINER_HOST=unix:///tmp/revm-containers.sock
podman run --rm alpine uname -a
```

Volumes persist independently of sessions. Each Podman launch must explicitly select an existing `--volume NAME` or an external disk. Stop and session removal preserve the data. `--container-disk PATH` uses a user-owned external disk. Share host directories explicitly with `--mount`; home is not mounted automatically.

```sh
revm volume ls                    # path, in-use state, capacity and allocated space
revm volume ls --json
revm volume rm data   # explicit deletion; refuses an active volume
revm cache prune                  # only inactive resources with known ownership
```

## Files and storage

Runtime sockets use a short private runtime directory. Rootfs copies use `${XDG_CACHE_HOME:-~/.cache}/revm/instances`; logs and results use `${XDG_STATE_HOME:-~/.local/state}/revm/sessions`; persistent volumes use `${XDG_DATA_HOME:-~/.local/share}/revm/volumes`. Roots cannot overlap. See [resource ownership](docs/resources.md).

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

Manual forwards support TCP/IPv4 and default to `127.0.0.1`. `--manage-api` and `--podman-api` expose convenient symlinks; unrelated existing paths are never overwritten.

## Development

```sh
go run ./scripts --build revm
# Unit and race tests (prepares pinned native dependencies automatically):
./scripts/test.sh
# Real VM checks on a host with virtualization support:
python3 tests/smoke.py
python3 tests/sessions.py
python3 tests/volumes.py
python3 tests/terminal.py
python3 tests/lifecycle.py
python3 tests/guest.py
```

Builds use a private source copy under `out/.build-*`; embedded files in the checkout are never modified. Dependencies are verified and cached by SHA-256 under `$XDG_CACHE_HOME/revm-build/v1` (or the OS cache directory). Different checkouts share immutable cache objects; concurrent builds in the same checkout are rejected. A failed build preserves the previous bundle. `--deps-dir PATH` uses local native archives with the same isolation.

The build downloads dependencies pinned by `deps.lock`, compiles the guest agent, signs the macOS executable, and creates a relocatable bundle and checksum. `go run ./scripts --lint` also runs lint. The dependency workflow builds libkrun, libkrunfw and Alpine from `deps/sources.lock`.

The manual `VM integration tests` workflow uses a dedicated self-hosted runner labeled `revm-vm`, with Go, Python, native build dependencies and access to KVM or Apple Hypervisor. Hosted build CI runs unit tests; the dedicated runner executes the real VM checks above.

See [session commands](docs/sessions.md), [exit status](docs/exit-status.md), [execution and signals](docs/execution-protocol.md), and the [command reference](docs/README.md).
