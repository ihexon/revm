# revm

## Install

Download the release archive for your platform, then extract the complete bundle:

```sh
# macOS: revm-darwin-arm64.tar.zst
# Linux: revm-linux-x86_64.tar.zst or revm-linux-aarch64.tar.zst
archive=revm-darwin-arm64.tar.zst
mkdir revm-release
tar -xf "$archive" -C revm-release
export PATH="$PWD/revm-release/bin:$PATH"
```

## Run

```sh
revm run -- sh
printf 'hello\n' | revm run -- cat
revm run --mount "$PWD:/workspace" --workdir /workspace -- sh
revm run --raw-disk "$HOME/data.ext4,mnt=/data" -- sh
```

## Sessions

```sh
# Terminal 1
revm run --id dev -- sh

# Terminal 2
revm ls
revm inspect dev
revm exec --id dev -- uname -a
revm logs --guest --tail 50 dev
revm stop dev
revm wait dev
revm rm dev
```

## Containers

```sh
# Terminal 1
revm volume create data
revm podman --volume data --id containers --podman-api /tmp/revm-containers.sock

# Terminal 2
export CONTAINER_HOST=unix:///tmp/revm-containers.sock
podman run --rm alpine uname -a
```

## Storage

```sh
revm volume ls
revm stop containers
revm volume rm data
revm cache prune
```

## Port forwarding

```sh
# Terminal 1
revm run --id demo --port-export 8080:8000 -- nc -lk -p 8000 -e cat

# Terminal 2
revm ports --json demo
printf 'hello\n' | nc -w 1 127.0.0.1 8080
```

## Completion and help

```sh
source <(revm completion bash)  # Bash with bash-completion
source <(revm completion zsh)   # Zsh with compinit
revm completion fish > ~/.config/fish/completions/revm.fish

revm --help
revm run --help
```

[Command reference](docs/README.md)
