# dockerd mode

[English](./dockerd.en.md)

`dockerd` 模式用于在本机启动一个轻量的 Linux 容器运行环境。你可以继续使用熟悉的 Docker CLI 或 Podman CLI，但容器运行在独立的 Linux 环境中。

它适合希望保留容器开发体验，同时减少对重型桌面容器产品依赖的团队。

## 适合谁

- 想用 Docker CLI，但希望容器运行环境更轻量的开发者。
- 需要在本机稳定运行 Linux 容器的 macOS 或 Linux 用户。
- 想把容器运行能力嵌入自研开发工具、CI 辅助工具或本地平台的团队。
- 希望容器状态、项目目录、代理和日志都能被命令行清晰控制的工程团队。

## 典型场景

### 场景 1：用 Docker CLI 启动隔离容器

启动运行环境：

```bash
./dockerd --id dev --podman-api /tmp/dockerd-dev.sock
```

在另一个终端使用 Docker CLI：

```bash
export DOCKER_HOST=unix:///tmp/dockerd-dev.sock
docker run --rm hello-world
```

你不用改变使用习惯，就能获得一个独立的 Linux 容器环境。

### 场景 2：本地开发项目需要容器

把项目目录挂载进运行环境，然后构建镜像：

```bash
./dockerd --id app \
  --podman-api /tmp/dockerd-app.sock \
  --mount "$PWD:/workspace"
```

```bash
export DOCKER_HOST=unix:///tmp/dockerd-app.sock
docker build -t app /workspace
docker run --rm app
```

适合本地服务开发、镜像构建、依赖验证和团队统一开发环境。

### 场景 3：保留容器镜像和数据

如果你不想每次重新拉镜像或丢失容器数据，可以指定持久化磁盘：

```bash
./dockerd --id dev --container-disk ~/.cache/dockerd-container.ext4
```

这让容器开发环境既可以独立运行，也可以保留长期使用的状态。

### 场景 4：测试 Web 服务端口

容器里的端口可以正常发布到本机：

```bash
docker run --rm -p 8080:80 nginx
curl http://127.0.0.1:8080
```

适合验证 Web 服务、API 服务、前端构建产物和本地集成测试。

### 场景 5：给内部工具提供容器能力

你可以把 `dockerd` 包装进自己的开发平台或自动化工具里：

```bash
./dockerd --id ci \
  --podman-api /tmp/dockerd-podman.sock \
  --mount "$PWD:/workspace" \
  --log-level info
```

上层工具只需要连接这个 socket，就能获得一套稳定的容器执行能力。

## 快速开始

启动：

```bash
./dockerd --id dev --podman-api /tmp/dockerd-dev.sock
```

使用 Podman CLI：

```bash
export CONTAINER_HOST=unix:///tmp/dockerd-dev.sock
podman run --rm alpine uname -a
```

使用 Docker CLI：

```bash
export DOCKER_HOST=unix:///tmp/dockerd-dev.sock
docker ps
docker run --rm hello-world
```

## 核心能力

- Docker CLI / Podman CLI 兼容。
- 项目目录挂载，方便本地开发和镜像构建。
- 容器端口发布，方便测试服务。
- 可选持久化容器存储。
- 支持本机代理设置，方便拉取依赖和镜像。
- 支持日志和稳定接入方式，方便自动化集成。

## 推荐用法

日常开发可以固定一个 session：

```bash
./dockerd --id dev \
  --podman-api /tmp/dockerd-dev.sock \
  --mount "$PWD:/workspace"
```

如果希望容器状态长期保留，增加持久化磁盘：

```bash
./dockerd --id dev \
  --podman-api /tmp/dockerd-dev.sock \
  --mount "$PWD:/workspace" \
  --container-disk ~/.cache/dockerd-container.ext4
```

如果是团队工具集成，建议指定稳定 socket 路径：

```bash
./dockerd --id team \
  --podman-api /tmp/dockerd-podman.sock \
  --mount "$PWD:/workspace"
```
