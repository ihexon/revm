# revm run

[English](run.en.md)

```sh
revm run [选项] -- COMMAND [ARG...]
revm run -- uname -a
printf 'hello\n' | revm run -- cat
revm run --cpus 2 --memory 1024 -- sh
revm run --mount "$PWD:/work,ro" --workdir /work -- ls
revm run --envs CHECK=value -- sh -c 'echo "$CHECK"'
revm run --raw-disk "$HOME/data.ext4,mnt=/data" -- sh
revm run --envs http_proxy=http://host.containers.internal:8080 -- sh
revm run --tty=never -- sh -c 'echo hello; exit 42'
```

`--mount` 只接受 `HOST:/GUEST[,ro]`，目标必须是绝对路径且不能重复。
磁盘参数为 `PATH[,uuid=UUID][,mnt=/GUEST][,readonly=true][,directio=true][,sync=relaxed|full|none]`。
磁盘不存在时自动创建，已有磁盘保留。应用负责刷盘。

```sh
# 终端 1：保留 session 和日志
revm run --id dev -- sh

# 终端 2
revm exec --id dev -- uname -a
revm inspect dev
revm logs --guest --tail 50 dev
revm stop dev
revm wait dev
revm rm dev
```

省略 `--id` 时，成功运行会清理记录；失败运行保留日志和结果。
Ctrl-C 或 `revm stop` 立即结束 VM 进程。

```sh
revm run --id demo --port-export 8080:8000 -- nc -lk -p 8000 -e cat
revm ports demo  # 在另一终端查询
```

`--network` 可选 `gvisor`（默认）或 `tsi`：

- `gvisor` 使用虚拟网卡，通过 `--port-export` 转发端口，适合普通运行和 Podman。
- `tsi` 将 IPv4/IPv6 TCP 和出站 UDP socket 接到宿主网络；TCP 可通过 `127.0.0.1` / `::1` 访问宿主服务。启动时复制宿主 `/etc/resolv.conf`，跳过当前 libkrun 无法正确传递的带 `%接口` 的 IPv6 DNS 地址；至少需要一个不带 scope 的 DNS 地址。Unix socket 仍留在 guest 内。

```sh
# 直接监听宿主 TCP 端口，无需 --port-export
revm run --network tsi -- nc -lk -s 127.0.0.1 -p 8080 -e cat
# 在另一终端访问
printf 'hello\n' | nc -w 1 127.0.0.1 8080
```

TSI 中监听 `0.0.0.0` 会监听宿主所有 IPv4 接口。
当前上游 TSI 优先尝试 guest 网络：TCP 端口冲突可能回退为仅 guest 可访问的监听，已连接 UDP 访问回环地址可能留在 guest 内。宿主使用回环 DNS 时，这会影响使用已连接 UDP 的解析器；可在 guest `/etc/resolv.conf` 中改用宿主可达的非回环 DNS 地址。
TSI 不接受 `--port-export`，没有可供 `revm ports` 查询的转发表，`podman` 命令仍使用 gvisor。
[上游 TSI](https://github.com/libkrun/libkrun/blob/a4d501ef87f7456a8b6632859a8575c16ee20f6b/README.md#virtio-vsock--tsi) 不支持 raw socket（如 ping）和 guest UDP 服务端；需要这些功能时使用 gvisor。

CPU 默认为宿主 CPU 数与 4 的较小值；内存默认为可用内存的四分之一，限制在 512–2048 MiB。
使用 `revm run --help` 查看全部选项。
