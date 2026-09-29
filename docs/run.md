# revm run

[English](run.en.md)

revm run 启动一个新的 session，使用程序内置的 Alpine rootfs，在 guest 内执行一条命令。它适合构建、测试、脚本、一次性 Linux 工具和需要隔离环境的本地任务。

## 语法

~~~text
revm run --id <session-id> [options] -- <command> [args...]
~~~

--id 必填。-- 后面的参数原样组成 guest 命令；不应把宿主 shell 语法直接交给 revm，复杂逻辑请显式使用 sh -c。

~~~bash
revm run --id shell -- sh -c 'uname -a; cat /etc/os-release'
revm run --id build --mount "$PWD:/workspace" --workdir /workspace -- sh -c 'make test'
~~~

命令退出后，revm 请求 guest 关闭并退出。要保持 session 存活，请在 guest 中运行长期进程，或使用 revm dockerd。

## rootfs 策略

VM rootfs 固定来自 release 内置的 Alpine 压缩包。它包括：

- Alpine 基础用户空间和 apk 包管理器
- guest-agent
- dropbear SSH 兼容服务
- Podman、containers 配置和容器网络工具
- 网络、挂载、日志和生命周期所需工具

run 不支持 --rootfs，也不支持替换、导入或导出 VM rootfs。需要其他发行版或自定义用户空间时，应启动 revm dockerd，再通过 Podman 运行对应镜像；这样 workload 不会改变 VM 控制面。

## 资源和网络

~~~bash
revm run --id test --cpus 4 --memory 4096 -- sh -c './test.sh'
revm run --id net --network gvisor -- sh
revm run --id light --network tsi -- sh
~~~

选项：

| 选项 | 说明 |
| --- | --- |
| --cpus | vCPU 数量。未设置或小于 1 时使用主机 CPU 数量；最大 32。 |
| --memory | 内存 MB。未设置时使用主机总内存；最小 512。 |
| --network | gvisor 或 tsi，默认 gvisor。 |
| --workdir | guest 命令的工作目录，默认 /。 |
| --envs KEY=VALUE | 传入 guest 命令的环境变量，可重复。 |
| --system-proxy | 读取 macOS 系统 HTTP/HTTPS 代理并传入 guest。gvisor 下会改写 127.0.0.1 代理地址。 |

gvisor 使用 gvisor-tap-vsock，提供 DNS、NAT、TCP/UDP 和端口转发。tsi 使用 libkrun 的透明 socket interception，启动路径更轻，但不能使用 ctl 的手动端口映射。

## VirtIO-FS 目录

使用 --mount 将宿主目录共享到 guest：

~~~text
--mount /host/path:/guest/path[,ro]
~~~

~~~bash
revm run --id files \
  --mount "$PWD:/workspace" \
  --mount "$HOME/.cache:/host-cache,ro" \
  --workdir /workspace \
  -- sh
~~~

共享目录中的已有文件在 guest 中保留宿主机 UID、GID 和权限。revm 不递归预处理外部共享目录。

如果 guest 需要读写目录，不要加 ro。mount 源路径会被解析为绝对路径，且必须位于用户 home 或 /tmp 下。

## 原始磁盘

--raw-disk 将 raw/ext4 磁盘挂载到 guest。镜像不存在时自动创建。

~~~text
--raw-disk <path>[,uuid=<uuid>][,version=<string>][,mnt=<guest-path>][,readonly=true][,directio=true][,sync=none|relaxed|full]
~~~

~~~bash
revm run --id disk \
  --raw-disk "$HOME/.cache/revm/data.ext4,version=v1,mnt=/data" \
  -- sh -c 'df -h /data'
~~~

说明：

- mnt 必须是绝对 guest 路径；省略时使用磁盘自身的默认挂载目标。
- version 写入并校验 user.vm.rawdisk.version；版本不一致时会重建镜像。
- readonly、directio 接受 true 或 false。
- sync 支持 none、relaxed、full。
- uuid 只在新建镜像时用于指定文件系统 UUID；已存在镜像不会强制改 UUID。

## 日志、socket 和 SSH key

默认 session 目录：

~~~text
~/.cache/revm/<session-id>/
~~~

默认日志：

~~~text
~/.cache/revm/<session-id>/logs/revm.log
~~~

~~~bash
revm run --id build --log-level debug --log-to /tmp/revm-build.log -- sh -c 'make test'
revm run --id build --manage-api /tmp/revm-build-vmctl.sock -- sh
revm run --id build --ssh-key /tmp/revm-build-ssh-key -- sh
~~~

--manage-api 和 --ssh-key 创建指向 session 内部文件的符号链接。管理 API 供 attach 和 ctl 使用；普通命令通过 guest-control vsock 执行，SSH 主要保留给 --pty 和兼容场景。

## 停止行为

第一次 SIGINT 或 SIGTERM 使用 libkrun 原生 shutdown 请求 guest 关闭。第二次信号、宿主 launcher 消失或 host service 失败会进入有界 force-stop，最长等待三秒。命令正常结束返回 guest 的退出结果；无法在有界时间内停止时返回错误。

## 全部选项

~~~text
--id
--cpus
--memory
--envs
--raw-disk
--mount
--system-proxy
--workdir
--network
--manage-api
--ssh-key
--report-events
--log-level
--log-to
~~~

执行 revm run --help 查看当前二进制的完整说明。
