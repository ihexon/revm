# docker 模式 — 你不需要安装 Docker Desktop 就可以迅速启动完整的 container 引擎

revm 内建完整的容器引擎，并通过 Unix socket 暴露给 podman-cli/docker-cli 调用，无需安装 Docker Desktop 或 Podman Desktop，即可快速启动完整的轻量容器软件栈。

## 快速开始

**启动容器引擎**

```bash
revm docker --id my-engine
```

启动后 Podman API socket 暴露在 `/tmp/my-engine/socks/podman-api.sock`。

**用 podman 或 docker CLI 连接**

```bash
export CONTAINER_HOST=unix:///tmp/my-engine/socks/podman-api.sock

# 查看运行环境信息
podman info

# 运行容器（与 docker 命令完全兼容）
podman run --rm ubuntu:latest uname -r
podman run --rm -it alpine:edge sh
podman run --rm -p 8080:80 nginx
```

**用 docker CLI 连接**

```bash
export DOCKER_HOST=unix:///tmp/my-engine/socks/podman-api.sock
docker run --rm hello-world
```

## 端口映射

docker 模式下，容器的端口映射（`-p`）通过 gvproxy 自动转发到 macOS 本机：

```bash
podman run --rm -p 8888:80 nginx
# macOS 上直接访问
curl http://127.0.0.1:8888
```

## 挂载宿主机目录

```bash
podman run --rm -v /Users/me/data:/data ubuntu:latest ls /data
```

## 系统代理透传

在需要代理访问网络的环境下，加上 `--system-proxy` 自动读取 macOS 系统代理并注入到容器内：

```bash
revm docker --id my-engine --system-proxy

# 容器内 apt/curl 自动走代理
podman run --rm ubuntu:latest apt-get update
```

## 参数列表

```bash
revm docker [flags]
```

| 参数                 | 说明                                                               | 默认值                   |
|--------------------|--------------------------------------------------------------------|-------------------------|
| `--id`             | **必填。** 会话 ID，会话目录由此派生为 `/tmp/<id>`；同名会话通过 flock 互斥 | — |
| `--cpus`           | 分配的 vCPU 核心数；不指定或小于 1 时自动取宿主机核心数                                 | 宿主机核心数                |
| `--memory`         | VM 内存大小（MB）；最小 512 MB；不指定时自动取宿主机可用内存                             | 宿主机可用内存               |
| `--envs`           | 传入环境变量（格式：`KEY=VALUE`，可重复）                                              | —                     |
| `--mount`          | 通过 VirtIO-FS 挂载宿主机目录（格式：`/host:/guest[,ro]`，可重复）                 | —                     |
| `--raw-disk`       | 挂载 ext4 裸盘镜像，不存在时自动创建镜像（可重复）                                     | —                     |
| `--network`        | 网络栈：`gvisor`（完整虚拟网卡，支持端口映射）或 `tsi`（透明转发）                         | `gvisor`              |
| `--system-proxy`   | 读取 macOS 系统代理并注入容器内，自动将 127.0.0.1 重写为 `host.containers.internal` | `false`               |
| `--container-disk` | 持久化容器存储磁盘路径（ext4 裸盘镜像）；不存在时自动创建；不指定则使用会话目录内的默认磁盘 | 会话目录内默认磁盘 |
| `--podman-proxy-api-file` | Podman API socket 的自定义 Unix socket 路径；默认为 `<会话目录>/socks/podman-api.sock` | —                     |
| `--manage-api-file` | VM 管理 API socket 的自定义 Unix socket 路径；默认为 `<会话目录>/socks/vmctl.sock` | —                     |
| `--ssh-key-dir`    | SSH 密钥对（`key` 和 `key.pub`）的符号链接目录；密钥始终在会话目录内生成                   | —                     |
| `--export-ssh-private-key` | SSH 私钥的符号链接文件路径                                                 | —                     |
| `--export-ssh-public-key`  | SSH 公钥的符号链接文件路径                                                 | —                     |
| `--log-level`      | 日志级别：`trace`、`debug`、`info`、`warn`、`error`、`fatal`、`panic`      | `info`                |
| `--log-to`         | 自定义日志文件路径；默认为 `<会话目录>/logs/vm.log`                                | 会话目录内                 |
| `--report-events-to` | 接收 VM 生命周期事件的 HTTP 端点（如 `unix:///var/run/events.sock` 或 `tcp://host:port`） | —                     |

docker 模式与 chroot 模式共用大部分参数，可按需灵活配置。

## 另请参阅

- [会话工作区与网络](insider_zh.md) — 会话目录结构、复用/清理，以及网络模式（gvisor / tsi）
