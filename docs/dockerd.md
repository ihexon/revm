# revm dockerd

[English](./dockerd.en.md)

`revm dockerd` 启动一个内置容器运行环境。它在 guest 内运行 Podman 服务，并在 host 上提供一个 Docker-compatible API socket，因此可以用 Docker CLI 或 Podman CLI 连接。

## 基本用法

```bash
revm dockerd --id <session-id> [flags]
```

启动容器 session：

```bash
revm dockerd --id dev --podman-api /tmp/revm-dev.sock
```

在另一个终端使用 Docker CLI：

```bash
export DOCKER_HOST=unix:///tmp/revm-dev.sock
docker run --rm hello-world
```

使用 Podman CLI：

```bash
export CONTAINER_HOST=unix:///tmp/revm-dev.sock
podman run --rm alpine uname -a
```

## API Socket

`--podman-api` 指定 host 上暴露的 Unix socket：

```bash
revm dockerd --id team --podman-api /tmp/revm-team.sock
```

不指定时，默认路径在 session 目录内：

```text
~/.cache/revm/<session-id>/socks/podman-api.sock
```

这个 socket 由 host 侧代理转发到 guest 内 Podman API。上层工具只需要连接这个 socket，不需要知道 VM 内部细节。

## 项目目录

挂载项目目录：

```bash
revm dockerd --id app \
  --podman-api /tmp/revm-app.sock \
  --mount "$PWD:/workspace"
```

构建镜像：

```bash
export DOCKER_HOST=unix:///tmp/revm-app.sock
docker build -t app /workspace
docker run --rm app
```

挂载格式：

```text
--mount /host/path:/guest/path[,ro]
```

## 容器存储

不指定 `--container-disk` 时，revm 使用 session 内默认容器存储盘。

指定持久化存储盘：

```bash
revm dockerd --id dev \
  --podman-api /tmp/revm-dev.sock \
  --container-disk ~/.cache/revm/container-storage.ext4
```

格式：

```text
--container-disk <path>[,version=<string>]
```

如果文件不存在，revm 会创建它。如果磁盘保存的版本缺失或与 `version` 不一致，revm 会重新创建该磁盘。适合把容器存储当成可重建缓存管理。

## 端口发布

容器自己的端口发布继续使用 Docker 或 Podman CLI：

```bash
export DOCKER_HOST=unix:///tmp/revm-dev.sock
docker run --rm -p 8080:80 nginx
curl http://127.0.0.1:8080
```

guest agent 会配置 Podman machine marker，使容器 start/stop 时调用 gvproxy 的 expose/unexpose API。

如果要手动暴露 guest 内的任意服务端口，使用 `revm ctl`：

```bash
revm ctl --id dev --list-port
revm ctl --id dev --port-export 127.0.0.1:8081:8081
revm ctl --id dev --port-unexport 127.0.0.1:8081
```

`--list-port` 会展示 SSH、容器发布端口和手动暴露端口。

## 资源、代理和日志

资源配置：

```bash
revm dockerd --id dev \
  --cpus 4 \
  --memory 4096 \
  --podman-api /tmp/revm-dev.sock
```

复用 macOS 系统代理：

```bash
revm dockerd --id dev --system-proxy --podman-api /tmp/revm-dev.sock
```

指定日志：

```bash
revm dockerd --id dev \
  --log-level debug \
  --log-to /tmp/revm-dockerd.log \
  --podman-api /tmp/revm-dev.sock
```

默认日志路径：

```text
~/.cache/revm/<session-id>/logs/revm.log
```

## Attach 和控制

连接到运行中的容器 session：

```bash
revm attach --id dev --pty
revm attach --id dev -- sh -c 'podman ps'
```

导出管理 API socket：

```bash
revm dockerd --id dev \
  --manage-api /tmp/revm-dev-vmctl.sock \
  --podman-api /tmp/revm-dev.sock
```

`revm attach` 通过管理 API 获取 SSH 信息并连接 guest。`revm ctl` 通过管理 API 获取 gvproxy endpoint 并执行控制面更新。
