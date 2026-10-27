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

`--network` accepts `gvisor` (default) or `tsi`:

- `gvisor` uses a virtual interface and `--port-export` forwarding, for ordinary runs and Podman.
- `tsi` connects IPv4/IPv6 TCP and outbound UDP sockets through the host network. TCP can reach host services through `127.0.0.1` / `::1`. The host's `/etc/resolv.conf` is copied at boot, excluding scoped IPv6 DNS addresses (`%interface`) that the pinned libkrun cannot forward correctly; at least one unscoped DNS address is required. Unix sockets stay inside the guest.

```sh
# Listen directly on a host TCP port, without --port-export
revm run --network tsi -- nc -lk -s 127.0.0.1 -p 8080 -e cat
# In another terminal
printf 'hello\n' | nc -w 1 127.0.0.1 8080
```

Binding `0.0.0.0` in TSI listens on all host IPv4 interfaces.
The current upstream TSI tries the guest network first: a TCP port conflict may fall back to a guest-only listener, and connected UDP to loopback may stay inside the guest. Host loopback DNS servers therefore affect resolvers using connected UDP; use a host-reachable non-loopback DNS address in the guest /etc/resolv.conf for those resolvers.
TSI rejects `--port-export` and has no forwarding table for `revm ports`. The `podman` command still uses gvisor.
[Upstream TSI](https://github.com/libkrun/libkrun/blob/a4d501ef87f7456a8b6632859a8575c16ee20f6b/README.md#virtio-vsock--tsi) does not support raw sockets (such as ping) or guest UDP servers; use gvisor for those workloads.

Defaults: up to four CPUs and one quarter of available memory, bounded to 512–2048 MiB.
Use `revm run --help` for all options.
