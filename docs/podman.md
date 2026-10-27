# revm podman

[English](podman.en.md)

```sh
# 终端 1
revm volume create data
revm podman --id containers --volume data --podman-api /tmp/revm-podman.sock

# 终端 2
export CONTAINER_HOST=unix:///tmp/revm-podman.sock
podman info
podman run --rm -p 8080:80 docker.io/library/nginx
revm ports containers
```

也可以直接指定外部磁盘；缺失时创建，已有磁盘保留：

```sh
revm podman --id containers --container-disk "$HOME/containers.ext4"
revm inspect containers  # 在另一终端查询 Podman API 地址
```

`--volume` 与 `--container-disk` 必须二选一。volume 独立于 session，删除 session 不会删除卷。
`--podman-api` 创建自定义连接地址，拒绝覆盖已有文件。

```sh
revm exec --id containers -- podman ps
revm logs --guest containers
revm stop containers
revm rm containers
revm volume ls
revm volume rm data
```

guest 普通服务使用启动参数 `--port-export HOST_PORT:GUEST_PORT`，容器使用 `podman -p`。
VM 在前台运行，Ctrl-C 或 `revm stop` 立即终止；应用负责刷盘。
使用 `revm podman --help` 查看 CPU、内存、挂载和环境变量选项。
