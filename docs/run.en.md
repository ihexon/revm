# revm run

```sh
revm run [options] -- COMMAND [ARG...]
revm run -- uname -a
printf 'hello\n' | revm run -- cat
revm run --cpus 2 --memory 1024 -- sh
revm run --mount "$PWD:/work,ro" --workdir /work -- ls
revm run --envs CHECK=value -- sh -c 'echo "$CHECK"'
revm run --raw-disk "$HOME/data.ext4,mnt=/data" -- sh
revm run --envs http_proxy=http://host.containers.internal:8080 -- sh
revm run --tty=never -- sh -c 'echo hello; exit 42'
```

Mounts accept only `HOST:/GUEST[,ro]`; guest targets must be absolute and unique.
Disk syntax: `PATH[,uuid=UUID][,mnt=/GUEST][,readonly=true][,directio=true][,sync=relaxed|full|none]`.
Missing disks are created; existing disks are preserved. Applications must flush their writes.

```sh
# Terminal 1: retain session records and logs
revm run --id dev -- sh

# Terminal 2
revm exec --id dev -- uname -a
revm inspect dev
revm logs --guest --tail 50 dev
revm stop dev
revm wait dev
revm rm dev
```

Without `--id`, successful runs remove their records; failed runs retain diagnostics.
Ctrl-C and `revm stop` immediately end the VM process.

```sh
revm run --id demo --port-export 8080:8000 -- nc -lk -p 8000 -e cat
revm ports demo  # In another terminal
```

Defaults: up to four CPUs and one quarter of available memory, bounded to 512–2048 MiB.
Use `revm run --help` for all options.
