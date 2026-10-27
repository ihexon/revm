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

CPU 默认为宿主 CPU 数与 4 的较小值；内存默认为可用内存的四分之一，限制在 512–2048 MiB。
使用 `revm run --help` 查看全部选项。
