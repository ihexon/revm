# 工作区与网络

## 工作区结构

`--workspace` 目录下的文件：

```
$WORKSPACE/
├── socks/
│   ├── podman-api.sock   # Podman API socket（docker 模式）
│   ├── gvpctl.sock       # gvproxy 控制 socket（gvisor 模式）
│   ├── vnet.sock         # 虚拟网络 socket（gvisor 模式）
│   ├── vmctl.sock        # VM 管理 API socket
│   └── ign.sock          # Ignition 配置服务 socket
├── ssh/
│   └── private.key       # 自动生成的 SSH 私钥
├── logs/
│   └── vm.log            # VM 内部日志
├── rootfs/               # 客户机根文件系统（chroot 模式）
└── raw-disk/             # 容器存储磁盘（docker 模式）
```

### 复用与清理

**复用**：下次启动时指定同一个 `--workspace`，VM 会直接加载上次的状态——容器镜像、卷数据、rootfs、SSH key 全部保留，无需重新配置或重新拉取镜像：

```bash
# 第一次启动
revm docker --workspace ~/revm_workspace

# 下次启动，镜像和数据原样保留
revm docker --workspace ~/revm_workspace
```

**临时环境**：不指定 `--workspace` 时，revm 自动使用 `/tmp/.revm-<random>` 临时目录，VM 退出后可安全删除，适合一次性任务或 CI 场景。

**清理**：删除整个 workspace 目录即可彻底重置，下次启动会创建全新环境：

```bash
rm -rf ~/revm_workspace
```

## 网络模式（ TSI/GVISOR 互斥）

docker 模式和 chroot 模式都支持 TSI 和 GVISOR 两种网络模式，这两种模式是互斥的

### gvisor（默认）

使用 [gvisor-tap-vsock](https://github.com/containers/gvisor-tap-vsock) 用户态网络栈。VM 固定 IP 为 `192.168.127.2`，网关为
`192.168.127.1`。容器内可通过 `host.containers.internal` 访问宿主机服务（如本机代理、本机 API 等）。

### tsi（透明 socket 拦截）

TSI（Transparent Socket Impersonation）是 libkrun 内置的网络模式，**不创建虚拟网卡**，guest 与 host 直接共享网络——两者可互相访问对方的 TCP/UDP 服务，无需特殊 IP 或端口转发规则。

与 gvisor 相比：**不支持 `-p` 端口映射**（无 gvproxy），也不提供 `host.containers.internal` 域名。在 TSI 模式下运行容器时，使用 `podman run --network=host` 可让容器直接复用 host 网络，端口在 macOS 上可直接访问，无需任何额外映射。
