# revm ctl

[English](ctl.en.md)

revm ctl 只控制一个正在运行 session 的 host-side 网络控制面。它不启动 VM，不执行 guest 命令，也不修改 rootfs。

当前操作只有三类：

- --list-port：列出当前 gvisor 端口映射。
- --port-export：将 guest TCP 端口暴露到 host。
- --port-unexport：移除 host 端口暴露。

连接 guest 使用 [revm attach](attach.md)。rootfs 不属于 ctl 的职责，rootfs 导入、导出和 --rootfs 参数已经删除。

## 语法

~~~text
revm ctl --id <session-id> --list-port
revm ctl --id <session-id> --port-export <spec> [--port-export <spec>...]
revm ctl --id <session-id> --port-unexport <spec> [--port-unexport <spec>...]
~~~

--id 必须指向正在运行且使用 gvisor 网络的 session。一次命令只能选择 list 或端口更新；端口更新可以同时包含 export 和 unexport。

## 列出端口

~~~bash
revm ctl --id web --list-port
~~~

输出示例：

~~~text
PROTOCOL  HOST            GUEST
tcp       127.0.0.1:6123  192.168.127.2:22
tcp       127.0.0.1:8080  192.168.127.2:8000
~~~

列表可能包括：

- revm 为 attach 建立的内部 SSH forwarding；
- Podman 容器通过 -p 发布的端口；
- ctl 手动创建的端口。

列表来源是 gvproxy；没有运行中的 gvisor session 时命令会失败。

## 暴露和取消暴露

将 guest 8000 暴露为 host 8080：

~~~bash
revm ctl --id web --port-export 127.0.0.1:8080:8000
curl http://127.0.0.1:8080
~~~

host IP 省略时默认为 127.0.0.1：

~~~bash
revm ctl --id web --port-export 8080:8000
~~~

取消暴露只需要 host 端点：

~~~bash
revm ctl --id web --port-unexport 127.0.0.1:8080
~~~

一次更新多个端口：

~~~bash
revm ctl --id web \
  --port-export 127.0.0.1:8080:8000 \
  --port-export 127.0.0.1:8443:8443 \
  --port-unexport 127.0.0.1:9000
~~~

格式：

~~~text
--port-export [tcp:]<host-port>:<guest-port>
--port-export [tcp:]<host-ip>:<host-port>:<guest-port>
--port-unexport [tcp:]<host-port>
--port-unexport [tcp:]<host-ip>:<host-port>
~~~

当前只接受 TCP 和 IPv4，端口范围为 1 到 65535。host 端口不能与 revm 内部 SSH forwarding 冲突，也不能在同一批更新中重复。

## 工作方式

ctl 通过 session 管理 API 读取 VM view：

~~~text
~/.cache/revm/<session-id>/socks/vmctl.sock
~~~

管理 API 返回 gvproxy control endpoint。ctl 随后调用 gvproxy forwarder 的 expose 或 unexpose API。guest 不需要运行额外的控制命令。

## 错误用法

没有操作会失败：

~~~bash
revm ctl --id dev
~~~

ctl 不执行 guest 命令：

~~~bash
revm ctl --id dev -- sh
~~~

应该使用：

~~~bash
revm attach --id dev -- sh
~~~

tsi session 不支持 list 或 port update：

~~~bash
revm run --id light --network tsi -- sh
revm ctl --id light --list-port
~~~

## 选项

| 选项 | 说明 |
| --- | --- |
| --id | 必填 session 名称。 |
| --list-port | 列出 gvisor 映射。 |
| --port-export | 创建 TCP/IPv4 映射，可重复。 |
| --port-unexport | 删除 TCP/IPv4 映射，可重复。 |
| --log-level | 日志等级。 |
| --log-to | 自定义 host 日志文件。 |
