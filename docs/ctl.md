# revm ctl

[English](./ctl.en.md)

`revm ctl` 控制已有 session 的控制面。它不会启动新 VM，也不会执行 guest 命令。

当前支持的控制操作：

- `--list-port`: 展示当前 gvproxy 端口映射。
- `--port-export`: 暴露 guest TCP 端口到 host。
- `--port-unexport`: 取消 host 端口暴露。
- `--export-rootfs`: 将 session 目录中的 rootfs 导出为 host 上的 tar.zst 文件。

连接 guest 或执行命令请使用 [`revm attach`](./attach.md)。

## 基本用法

```bash
revm ctl --id <session-id> --list-port
revm ctl --id <session-id> --port-export <spec>
revm ctl --id <session-id> --port-unexport <spec>
revm ctl --id <session-id> --export-rootfs <path.tar.zst>
```

`--id` 必须指向一个正在运行的 session。

## 导出 rootfs

将 session 目录中的 rootfs 导出为 tar.zst：

```bash
revm ctl --id web --export-rootfs ./rootfs.tar.zst
```

tar 内容以 rootfs 内容为根，不会额外包含一层 `rootfs/` 目录。导出目标不能放在该 session 的 rootfs 目录内。

## 展示端口

展示当前所有端口映射：

```bash
revm ctl --id web --list-port
```

输出包括 revm 内部 SSH forward、容器端口发布和手动暴露的端口：

```text
PROTOCOL  HOST            GUEST
tcp       127.0.0.1:6123  192.168.127.2:22
tcp       127.0.0.1:8080  192.168.127.2:8000
```

## 暴露端口

暴露 guest 端口：

```bash
revm ctl --id web --port-export 127.0.0.1:8080:8000
curl http://127.0.0.1:8080
```

取消暴露：

```bash
revm ctl --id web --port-unexport 127.0.0.1:8080
```

可以一次更新多个端口：

```bash
revm ctl --id web \
  --port-export 127.0.0.1:8080:8000 \
  --port-export 127.0.0.1:8443:8443
```

端口格式：

```text
--port-export [tcp:]<host-port>:<guest-port>
--port-export [tcp:]<host-ip>:<host-port>:<guest-port>
--port-unexport [tcp:]<host-port>
--port-unexport [tcp:]<host-ip>:<host-port>
```

未指定 host IP 时默认使用 `127.0.0.1`。当前只支持 TCP 和 IPv4。

## 工作方式

`revm ctl` 先通过 session 的管理 API 读取 VM 信息：

```text
~/.cache/revm/<session-id>/socks/vmctl.sock
```

端口更新会从管理 API 获取 gvproxy control endpoint，然后调用 gvproxy forwarder API：

```text
/services/forwarder/expose
/services/forwarder/unexpose
```

因此端口更新要求 VM 使用 gvisor 网络。`tsi` 网络不支持 `--port-export` 或 `--port-unexport`。

## 错误用法

下面的命令会失败，因为没有选择控制操作：

```bash
revm ctl --id dev
```

下面的命令会失败，因为 `ctl` 不执行 guest 命令：

```bash
revm ctl --id dev -- sh
```

应该改用：

```bash
revm attach --id dev -- sh
```
