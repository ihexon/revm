# revm ports

[English](ports.en.md)

```sh
revm ports SESSION
revm ports --json SESSION
```

列出运行中 session 的当前 TCP 端口映射，包括启动时配置的转发和 Podman 发布的端口。

```sh
# 终端 1：HOST_PORT:GUEST_PORT，默认只监听 127.0.0.1
revm run --id demo --port-export 8080:8000 -- nc -lk -p 8000 -e cat

# 终端 2
revm ports demo
printf 'hello\n' | nc -w 1 127.0.0.1 8080
```

`--port-export` 可重复指定，也接受 `HOST_IP:HOST_PORT:GUEST_PORT`；支持 TCP 和 IPv4。
修改启动转发规则需重启 session。容器端口使用 `podman run -p HOST_PORT:CONTAINER_PORT`。

TSI 直接监听宿主端口，不使用转发表。`--port-export` 和 `revm ports` 仅用于 gvisor 网络。
