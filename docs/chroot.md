# chroot mode

[English](./chroot.en.md)

`chroot` 模式用于快速运行一个隔离的 Linux 命令环境。你可以把它理解成“更适合开发工作的 chroot”：使用方式像本地命令，运行环境更干净，也更适合自动化。

## 适合谁

- 想在干净 Linux 环境里跑构建和测试的开发者。
- 想快速打开一次性 Linux Shell 的工程师。
- 想检查 rootfs、验证脚本、复现用户环境的工具开发者。
- 想给 CI、本地开发平台或沙箱执行能力找一个轻量底座的团队。

## 典型场景

### 场景 1：项目构建环境不想污染本机

把当前项目目录挂载进去，然后在 Linux 环境里跑测试：

```bash
./chroot --id build \
  --mount "$PWD:/workspace" \
  --workdir /workspace \
  -- sh -c 'make test'
```

适合解决这些问题：

- 本机依赖太多，构建结果不稳定。
- 团队成员系统不同，测试结果不一致。
- 只想为某个任务创建一个干净环境，用完就退出。

### 场景 2：临时调试 Linux 命令或脚本

打开一个 Shell：

```bash
./chroot --id debug -- sh
```

你可以在里面验证命令、跑脚本、检查 Linux 行为，不需要长期维护一台 VM。

### 场景 3：使用自己的 rootfs 复现环境

如果你已经准备好了一个 rootfs，可以直接指定：

```bash
./chroot --id ubuntu --rootfs ~/ubuntu-rootfs -- bash
```

这很适合复现特定发行版、特定依赖集合或某个线上问题环境。

### 场景 4：在自动化流程中执行任务

把它放进脚本或 CI 辅助工具里：

```bash
./chroot --id ci \
  --cpus 4 \
  --memory 4096 \
  --mount "$PWD:/src,ro" \
  --workdir /src \
  -- sh -c './ci/test.sh'
```

这样团队可以把“准备环境”和“执行任务”合并成稳定、可重复的一步。

## 快速开始

使用内置 Linux 环境执行命令：

```bash
./chroot --id quick -- sh -c 'uname -a && cat /etc/os-release'
```

进入交互式 Shell：

```bash
./chroot --id shell -- sh
```

挂载当前项目目录：

```bash
./chroot --id dev \
  --mount "$PWD:/workspace" \
  --workdir /workspace \
  -- sh
```

## 核心能力

- 使用内置 Linux 环境，也可以切换成团队自己的 rootfs。
- 把本机项目目录挂载进去，直接在隔离环境里执行构建和测试。
- 为不同项目固定工作目录、环境变量和资源大小。
- 复用本机代理设置，方便下载依赖。
- 输出日志，便于接入脚本、CI 或内部平台。

## 推荐用法

如果只是临时验证，直接使用内置环境：

```bash
./chroot --id test -- sh
```

如果是团队构建或测试，建议固定 rootfs、工作目录和资源配置：

```bash
./chroot --id project-test \
  --rootfs ~/project-rootfs \
  --cpus 4 \
  --memory 4096 \
  --mount "$PWD:/workspace" \
  --workdir /workspace \
  -- sh -c 'make test'
```

这样每个人都能用同一套命令得到更一致的结果。
