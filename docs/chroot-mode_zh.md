# chroot 模式 — macOS 上的 Linux chroot 替代方案

revm 的 chroot 模式通过启动一个真正的 Linux Kernel 来执行 Rootfs 内的二进制程序。由于底层使用 libkrun + Apple Hypervisor，
**启动时间通常在 1-2 秒以内**，近乎原生效率，隔离性更好，可直接在 macOS 上通过 `chroot` 模式进入任何 Rootfs。

## 典型使用场景

**快速运行任何 Rootfs**

```bash
# 用自己的 Ubuntu rootfs 做集成测试
revm chroot --id build --rootfs ~/ubuntu-jammy -- bash -c 'apt-get install -y libssl-dev && make test'

# 只需快速启动一个 Linux Shell，直接使用内置 Alpine Rootfs
revm chroot --id quick -- sh -c 'uname -r'
```

**挂载宿主机源码目录进行编译**

```bash
revm chroot --id compile \
  --rootfs ~/ubuntu-rootfs \
  --mount /Users/me/myproject:/workspace \
  --workdir /workspace \
  bash -c 'make && ./run_tests.sh'
```

**保持 VM 存活，在需要的时候交互式 attach 到已经运行中的 Rootfs**

```bash
# 终端 1：保持 VM 存活
revm chroot --id dev-env --rootfs ~/ubuntu-rootfs sleep 86400

# 终端 2：进入交互式 Shell
revm attach --pty dev-env

# 终端 3：执行一条命令
revm attach dev-env -- df -h
```

**挂载持久化数据盘**

```bash
# 第一次运行时自动创建 ext4 磁盘镜像，revm 会自动挂载到 /mnt/<UUID> 下
revm chroot --id disktest --raw-disk ~/data.ext4 sh -c 'mount'

# 下次运行时复用同一块盘，数据持久保留
revm chroot --id disktest --raw-disk ~/data.ext4 sh -c 'ls /mnt'
```

## 参数列表

```bash
revm chroot [flags] <command> [args...]
```

| 参数               | 说明                                                | 默认值                   |
|------------------|---------------------------------------------------|-----------------------|
| `--rootfs`       | rootfs 目录路径，须包含 `/bin/sh`；不指定则使用内置 Alpine         | 内置 Alpine             |
| `--id`           | **必填。** 会话 ID，会话目录由此派生为 `/tmp/<id>`；同名会话通过 flock 互斥 | — |
| `--cpus`         | 分配的 vCPU 核心数；不指定或小于 1 时自动取宿主机核心数                  | 宿主机核心数                |
| `--memory`       | VM 内存大小（MB）；最小 512 MB；不指定时自动取宿主机可用内存              | 宿主机可用内存               |
| `--workdir`      | 进入 VM 后执行命令前的工作目录                                 | `/`                   |
| `--mount`        | 通过 VirtIO-FS 挂载宿主机目录（格式：`/host:/guest[,ro]`，可重复）  | —                     |
| `--raw-disk`     | 挂载 ext4 裸盘镜像（格式：`<path>[,uuid=<uuid>][,version=<string>][,mnt=<guest-path>]`）；只传路径即可；新磁盘会自动创建，默认随机 UUID，并挂载到 `/mnt/<UUID>`（可重复） | — |
| `--envs`         | 传入环境变量（格式：`KEY=VALUE`，可重复）                        | —                     |
| `--network`      | 网络栈：`gvisor`（完整虚拟网卡）或 `tsi`（透明 socket 转发）         | `gvisor`              |
| `--system-proxy` | 读取 macOS 系统代理并以 `http_proxy`/`https_proxy` 注入到 VM | `false`               |
| `--manage-api-file` | VM 管理 API socket 的自定义 Unix socket 路径；默认为 `<会话目录>/socks/vmctl.sock` | —                     |
| `--ssh-key-dir`  | SSH 密钥对（`key` 和 `key.pub`）的符号链接目录；密钥始终在会话目录内生成   | —                     |
| `--export-ssh-private-key` | SSH 私钥的符号链接文件路径                                    | —                     |
| `--export-ssh-public-key`  | SSH 公钥的符号链接文件路径                                    | —                     |
| `--log-level`    | 日志级别：`trace`、`debug`、`info`、`warn`、`error`、`fatal`、`panic` | `info`          |
| `--log-to`       | 自定义日志文件路径；默认为 `<会话目录>/logs/vm.log`                  | 会话目录内                 |
| `--report-events-to` | 接收 VM 生命周期事件的 HTTP 端点（如 `unix:///var/run/events.sock` 或 `tcp://host:port`） | —               |

## 另请参阅

- [会话工作区与网络](insider_zh.md) — 会话目录结构、复用/清理，以及网络模式（gvisor / tsi）
