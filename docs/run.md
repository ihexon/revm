# revm run

[English](./run.en.md)

`revm run` 启动一个 Linux rootfs session，并在 guest 内执行命令。它适合构建、测试、脚本执行、一次性调试和需要干净 Linux 环境的本地工具。

## 基本用法

```bash
revm run --id <session-id> [flags] -- <command> [args...]
```

`--id` 是必填项。`--` 之后的内容会作为 guest 内命令执行。

```bash
revm run --id quick -- sh -c 'uname -a && cat /etc/os-release'
```

打开交互式 shell：

```bash
revm run --id shell -- sh
```

挂载当前项目并在 guest 中执行测试：

```bash
revm run --id build \
  --mount "$PWD:/workspace" \
  --workdir /workspace \
  -- sh -c 'make test'
```

## rootfs

不指定 `--rootfs` 时，`revm run` 使用内置 rootfs。

使用自定义 rootfs：

```bash
revm run --id ubuntu --rootfs ~/rootfs/ubuntu -- bash
```

自定义 rootfs 至少需要提供可执行的 `/bin/sh`。如果命令依赖其他工具，需要由 rootfs 自己提供。

## 资源

```bash
revm run --id test \
  --cpus 4 \
  --memory 4096 \
  -- sh -c './test.sh'
```

- `--cpus`: vCPU 数量。未设置或小于 1 时使用主机 CPU 数量。
- `--memory`: 内存大小，单位 MB。未设置时使用主机可用内存；最小值是 512。

## 文件与目录

共享目录使用 VirtIO-FS：

```bash
revm run --id dev \
  --mount "$PWD:/workspace" \
  --mount "$HOME/.cache/go-build:/go-cache,ro" \
  --workdir /workspace \
  -- sh
```

挂载格式：

```text
--mount /host/path:/guest/path[,ro]
```

原始 ext4 磁盘使用 `--raw-disk`：

```bash
revm run --id disk \
  --raw-disk ~/.cache/revm/data.ext4,mnt=/data,version=v1 \
  -- sh -c 'df -h /data'
```

磁盘格式：

```text
--raw-disk <path>[,uuid=<uuid>][,version=<string>][,mnt=<guest-path>]
```

如果文件不存在，revm 会创建它。`version` 用于区分磁盘内容版本，适合构建缓存和可重建数据。

## 环境变量与代理

传入环境变量：

```bash
revm run --id env \
  --envs GOPROXY=https://proxy.golang.org,direct \
  --envs CI=true \
  -- sh -c 'env | sort'
```

复用 macOS 系统代理：

```bash
revm run --id proxy --system-proxy -- sh -c 'curl -I https://example.com'
```

在 gvisor 网络模式下，指向 `127.0.0.1` 的系统代理会被改写成 guest 可访问的 host 地址。

## 网络

`revm run` 默认使用 gvisor 网络：

```bash
revm run --id net --network gvisor -- sh
```

可选值：

- `gvisor`: 使用 gvisor-tap-vsock，支持 NAT、DNS、端口暴露和容器场景。
- `tsi`: 使用 libkrun transparent socket interception，路径更轻，但不支持 `revm ctl --port-export`。

## 暴露 guest 端口

端口暴露由 `revm ctl` 操作已有 session，不在 `revm run` 启动路径里解析。

先启动一个长期运行的服务：

```bash
revm run --id web -- sh -c 'cd /tmp && python3 -m http.server 8000'
```

在另一个终端暴露端口：

```bash
revm ctl --id web --list-port
revm ctl --id web --port-export 127.0.0.1:8080:8000
curl http://127.0.0.1:8080
revm ctl --id web --port-unexport 127.0.0.1:8080
```

端口展示和端口更新都要求 session 使用 gvisor 网络。`--list-port` 会展示 SSH、容器发布端口和手动暴露端口。

## Attach

`revm attach` 可以连接到已有 `run` session：

```bash
revm attach --id web --pty
revm attach --id web -- sh -c 'ps aux'
```

`revm attach` 不会创建新 VM。如果 session 不存在，命令会失败。

## 日志与控制接口

默认日志路径：

```text
~/.cache/revm/<session-id>/logs/revm.log
```

显式指定日志：

```bash
revm run --id build --log-level debug --log-to /tmp/revm-build.log -- sh -c 'make test'
```

导出管理 API socket：

```bash
revm run --id build --manage-api /tmp/revm-build-vmctl.sock -- sh
```

管理 API 用于 `revm ctl` 获取 attach 信息和 gvproxy endpoint。
