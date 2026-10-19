# revm podman

[English](podman.en.md)

revm podman 启动一个长期运行的 Alpine VM，在 guest 内启动 Podman API 服务，并在宿主机提供一个 Unix socket。宿主机 Podman CLI 通过这个 socket 使用容器运行时。

revm podman 固定使用 gvisor 网络，因此支持容器端口发布和 revm ctl 端口控制。


## 基本用法

~~~text
revm podman --id <session-id> [options]
~~~

~~~bash
# 终端 1：启动 VM 和 Podman 服务。
revm podman --id dev --podman-api /tmp/revm-dev.sock
~~~

在另一个终端连接宿主机 Podman 客户端：

~~~bash
export CONTAINER_HOST=unix:///tmp/revm-dev.sock
podman run --rm alpine uname -a
~~~

服务会一直运行，直到收到停止信号或 host service 失败。停止 revm podman 后，容器也会失去其 guest runtime。

## API socket

--podman-api 是宿主机上暴露的 Unix socket 路径。未指定时：

~~~text
$XDG_RUNTIME_DIR/revm/<instance>/socks/podman-api.sock
~~~

这个 socket 经私有 vsock 通道连接 guest 内 Podman system service，原样传递 Podman 原生 API。客户端不需要访问 guest IP、vsock 或 SSH。传输复用 `mdlayher/vsock`、`libp2p/go-yamux/v5` 和 `inetaf/tcpproxy`；Yamux 提供独立的流控和输入半关闭，支持 `exec -i`、attach、build、日志和文件传输。

Podman 只监听 guest 内 `/run/revm/podman/podman.sock`，不开放 TCP 管理端口。目录权限为 `0700`，socket 为 `0600`。Podman 通道与 guest-control 都只接受来自宿主 CID 2 的连接，拒绝 guest 回环及使用宿主网络的普通容器。宿主端所有控制 socket 位于 `0700` 的实例 runtime 目录内，`--podman-api` 只是指向其中 socket 的符号链接。

普通工作负载不挂载管理 socket。需要管理容器的可信 Supervisor 可显式挂载 guest 的 `/run/revm/podman` 目录；挂载整个目录可在服务重启、socket 重建后继续连接。此权限等同于管理整个 VM 的容器引擎；特权容器或能够访问 guest 根目录的容器不属于隔离边界。

管理 API 可以单独导出：

~~~bash
revm podman --id dev \
  --podman-api /tmp/revm-dev.sock \
  --manage-api /tmp/revm-dev-vmctl.sock
~~~

## 项目目录和 home

--mount 共享项目目录：

~~~bash
revm podman --id app \
  --podman-api /tmp/revm-app.sock \
  --mount "$PWD:/workspace"
~~~

revm podman 还会自动把宿主 home 目录挂载到 guest 中的相同路径，以便 Podman workload 使用项目、凭证和缓存。共享目录中的已有文件在 guest 中保留宿主机 UID、GID 和权限；revm 不预处理外部共享目录。

挂载格式：

~~~text
--mount /host/path:/guest/path[,ro]
~~~

容器 workload 的 rootfs 由 Podman 镜像负责，不能通过 revm 的 --rootfs 替换 VM rootfs。

## 容器存储

容器存储位于 guest 的 /var/lib/containers，并由一个 ext4 raw disk 提供。

未指定 --container-disk 时，使用独立的 `podman-<session-id>` 持久卷：

~~~text
${XDG_DATA_HOME:-~/.local/share}/revm/volumes/<volume-id>/disk.ext4
~~~

使用持久化路径：

~~~bash
revm podman --id dev \
  --podman-api /tmp/revm-dev.sock \
  --container-disk "$HOME/.cache/revm/container-storage.ext4,version=containers-v1"
~~~

格式：

~~~text
--container-disk <path>[,version=<string>]
~~~

外部磁盘不存在时自动创建，之后仍由用户持有。已有磁盘缺少版本属性时检查原盘；版本不一致时报错并保留数据。停止或删除 session 不删除默认 volume，同名启动会重新连接原卷。显式删除使用 `revm volume rm NAME`，运行中的卷不可删除。旧布局需 `revm migrate ID`。

## 端口发布

容器端口使用 Podman 的 -p 选项：

~~~bash
export CONTAINER_HOST=unix:///tmp/revm-dev.sock
podman run --rm -d --name revm-web -p 127.0.0.1:8080:80 nginx
curl http://127.0.0.1:8080
podman stop revm-web
~~~

guest agent 为 Podman 配置 machine marker，使容器启动和停止时调用 gvisor forwarder 的 expose/unexpose。

guest 内非容器服务可由 ctl 手动暴露：

~~~bash
revm ctl --id dev --list-port
revm ctl --id dev --port-export 127.0.0.1:8081:8081
revm ctl --id dev --port-unexport 127.0.0.1:8081
~~~

ctl 端口格式只支持 TCP 和 IPv4；使用 tsi 的 run session 不支持这些操作。

## 资源、环境和代理

~~~bash
revm podman --id dev \
  --cpus 4 \
  --memory 4096 \
  --envs CI=true \
  --system-proxy \
  --podman-api /tmp/revm-dev.sock
~~~

| 选项 | 说明 |
| --- | --- |
| --cpus | vCPU 数量，默认 min(主机 CPU 数量, 4)，最大 32。 |
| --memory | 内存 MB，默认可用内存四分之一，限制在 512–2048 MB，最小 512。 |
| --envs KEY=VALUE | 传给 Podman service 和容器运行环境的环境变量，可重复。 |
| --system-proxy | 将 macOS 系统代理传入 guest。 |
| --raw-disk | 添加额外 raw/ext4 磁盘，可重复。 |
| --container-disk | 设置 Podman 容器存储盘。 |
| --mount | 添加 VirtIO-FS 共享目录，可重复。 |
| --podman-api | 自定义 Podman API socket。 |
| --manage-api | 自定义 VM 管理 socket。 |
| --ssh-key | 导出兼容 SSH key 的符号链接。 |
| --report-events | 接收生命周期事件的 HTTP endpoint。 |
| --log-level | 设置日志等级。日志始终写入 session 目录。 |

revm podman 没有 --network 选项，始终使用 gvisor。

## Attach 和 ctl

~~~bash
revm attach --id dev --pty
revm attach --id dev -- sh -c 'podman ps'
revm ctl --id dev --list-port
~~~

attach 使用 guest-control v2 执行普通命令和 PTY。ctl 只处理端口映射，不执行 guest 命令。

## 停止行为

第一次 SIGINT 或 SIGTERM 请求 libkrun 原生 shutdown；第二次信号或 host service 失败触发最长三秒的 force-stop。容器存储磁盘在正常停止时会同步。

See [resource ownership](resources.md) and [session management](sessions.md). Runtime paths may use the private `/tmp/revm-<uid>` fallback; logs and results are stored under XDG_STATE_HOME.
