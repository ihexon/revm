# 会话目录与网络

## 会话目录结构

每个会话的目录位于 `/tmp/<id>`，由 `--id` 参数派生（不指定时使用随机字符串）：

```
/tmp/<id>/
├── socks/
│   ├── podman-api.sock   # Podman API socket（docker 模式）
│   ├── gvpctl.sock       # gvproxy 控制 socket（gvisor 模式）
│   ├── vnet.sock         # 虚拟网络 socket（gvisor 模式）
│   ├── vmctl.sock        # VM 管理 API socket
│   └── ign.sock          # Ignition 配置服务 socket
├── ssh/
│   ├── key               # 自动生成的 SSH 私钥
│   └── key.pub           # 自动生成的 SSH 公钥
├── logs/
│   └── vm.log            # VM 内部日志
├── rootfs/               # 客户机根文件系统（chroot 模式）
└── raw-disk/             # 容器存储磁盘（docker 模式）
```

### 符号链接参数

以下参数会创建指向会话目录内部的符号链接，方便外部工具通过固定路径访问资源，同时不破坏会话目录的完整性：

| 参数               | 链接目标                                            |
|--------------------|-------------------------------------------------|
| `--podman-proxy-api-file` | `<会话目录>/socks/podman-api.sock`          |
| `--manage-api-file` | `<会话目录>/socks/vmctl.sock`                    |
| `--ssh-key-dir`    | `<会话目录>/ssh/key` 和 `<会话目录>/ssh/key.pub`     |
| `--export-ssh-private-key` | `<会话目录>/ssh/key`                       |
| `--export-ssh-public-key`  | `<会话目录>/ssh/key.pub`                   |

### 会话生命周期

会话目录是**临时的** — VM 退出后，`/tmp/<id>/` 会在清理阶段自动删除。每次启动都从全新目录开始。

**互斥**：同 ID 会话通过 flock 互斥——同一时间只有一个 VM 可以使用给定的 ID。因此 `--id` 适用于 `revm attach` 连接到正在运行的会话。

**持久化数据**：如需跨会话保留数据，请使用指向会话目录外部路径的显式参数：

```bash
# 容器镜像跨会话保留
revm docker --id my-engine --container-disk ~/container-storage.ext4

# 任意数据也可持久化
revm chroot --raw-disk ~/data.ext4 -- sh
```

**清理**：如果进程被强制杀死（例如 `kill -9`），手动移除残留会话目录：

```bash
rm -rf /tmp/my-engine
```

## 网络模式（TSI/GVISOR 互斥）

docker 模式和 chroot 模式都支持 TSI 和 GVISOR 两种网络模式，这两种模式是互斥的。

### gvisor（默认）

使用 [gvisor-tap-vsock](https://github.com/containers/gvisor-tap-vsock) 用户态网络栈。VM 固定 IP 为 `192.168.127.2`，网关为
`192.168.127.1`。容器内可通过 `host.containers.internal` 访问宿主机服务（如本机代理、本机 API 等）。

### tsi（透明 socket 拦截）

TSI（Transparent Socket Impersonation）是 libkrun 内置的网络模式，**不创建虚拟网卡**，guest 与 host 直接共享网络——两者可互相访问对方的 TCP/UDP 服务，无需特殊 IP 或端口转发规则。

与 gvisor 相比：**不支持 `-p` 端口映射**（无 gvproxy），也不提供 `host.containers.internal` 域名。在 TSI 模式下运行容器时，使用 `podman run --network=host` 可让容器直接复用 host 网络，端口在 macOS 上可直接访问，无需任何额外映射。
