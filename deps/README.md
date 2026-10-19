# 依赖构建

[English version](README.en.md)

revm 的 libkrun、libkrunfw 和 Alpine rootfs 不在普通应用构建中本地编译。它们由 GitHub Actions 构建为依赖 release，应用构建通过 deps.lock 下载并校验固定资产。

## 两层构建

1. 依赖构建：手动运行 .github/workflows/build-deps.yml。
2. 应用构建：运行 go run ./scripts --build revm，消费 deps.lock 指向的 release。

这样可以把需要特定 runner、Docker、Rust、交叉架构和虚拟化依赖的构建从日常 Go 开发中隔离出来。

## 依赖来源

deps/sources.lock 固定上游源：

- libkrun 2.x 开发线的固定 commit（当前应用使用 2.x API，不能直接回退到 1.x release）
- libkrunfw 的固定 commit
- gvisor-tap-vsock
- Alpine 版本和构建输入
- libarchive 及其他原生依赖

deps.lock 固定已经发布的资产名称、版本和 SHA-256。应用构建下载后会校验 checksum；资产缺失或校验不匹配会直接失败。

## 依赖 workflow

在 GitHub Actions 中手动触发 build-deps，并提供形如 deps-v20260928.1 的 release tag。workflow 会：

- 在 Linux amd64 和 arm64 runner 上构建 Alpine rootfs、libkrun 和 libkrunfw；
- 在 macOS arm64 runner 上构建 libkrun 和 libkrunfw；
- 为打包的 rootfs 写入 user.containers.override_stat 元数据；
- 生成 SHA256SUMS 和 manifest；
- 发布 deps-* GitHub release。

只有当 sources.lock、构建脚本、patch 或配置变化，或者需要升级上游 Head 时，才需要生成新的依赖 release。

## 目录

~~~text
deps/
  config/       libkrunfw 和 rootfs 配置
  docker/       Linux 依赖构建镜像
  patches/      构建时应用的补丁
  scripts/      各平台依赖构建脚本
  sources.lock  上游版本和 commit 锁定
~~~

deps/.work 和 deps/dist 是生成目录，已加入 gitignore。

## 应用构建

应用构建读取根目录 deps.lock：

~~~bash
go run ./scripts --build revm
~~~

应用构建只在 `out/.build-*` 副本中写入 rootfs 和 guest-agent，不修改源码中的嵌入文件。下载归档按校验和缓存，解压目录以 SHA-256 命名并在完成后发布；不同 checkout 可以共用缓存，同一 checkout 的构建由锁串行保护（重复启动直接报错）。缓存位于 `$XDG_CACHE_HOME/revm-build/v1`，未设置时使用系统缓存目录。构建失败保留上一次成功的 bundle。`./scripts/test.sh` 自行准备相同的原生依赖，无需先构建，也不依赖全局 `/tmp/.deps`。

它会下载当前平台的依赖资产，嵌入 guest-agent 和静态资源，链接 revm，并在 out/revm 生成 bundle。这个流程不会在本机重新编译 libkrun 或 libkrunfw。

## 本地验证新依赖

本地构建脚本输出到 `deps/dist`。无需发布 GitHub release 即可使用它们：

~~~bash
go run ./scripts --build revm --deps-dir deps/dist
~~~

指定目录必须同时包含当前平台的 libkrun 和 libkrunfw 压缩包；缺失时直接失败，不会退回旧版本。构建日志记录本地资产的 SHA-256，Alpine rootfs 仍使用 `deps.lock` 中校验过的版本。未指定 `--deps-dir` 时，仍消费 `deps.lock` 的已发布资产；修改 `sources.lock` 不会自动改变已发布资产。

Apple Silicon 可使用已有 revm 启动构建 VM，通过 Podman 在 `deps/docker/linux.Dockerfile` 的镜像内运行 `deps/scripts/build_libkrunfw_linux.sh`。将 `DEPS_WORK_DIR` 指向容器磁盘或命名 volume（内核源码需要区分大小写的文件系统），将仓库挂载到相同路径，以便产物写入宿主的 `deps/dist`。随后在 macOS 运行 `build_libkrun_darwin.sh`、`build_libkrunfw_darwin.sh`。设置 `DEPS_BUILD_JOBS` 控制内核并发，默认 4；Rust 并发使用 `CARGO_BUILD_JOBS`。

## 内核配置

以 `deps/config/config-libkrunfw_*` 为基础，应用上游 patches，然后用 Linux 自带的 `olddefconfig` 解析新内核依赖。`check_kernel_config.sh` 使用 `diffconfig` 对照同一内核规范化后的官方配置，并检查容器必需的存储、网络、namespace、cgroup、seccomp 和 vsock 功能。未审核的上游功能缺失会导致构建失败。

有意排除的官方功能只有 GPU 显示及其依赖、x86 的 32 位兼容支持。ARM64 保留 4 KB 页。构建产物的 `share/revm/kernel` 保存输入、最终配置和差异，应用 bundle 将它们收录到 `share/libkrunfw/kernel`；源码 commit 一并保存，便于核对实际使用的内核。

运行隔离与流式传输测试：

~~~bash
CONTAINER_HOST=unix:///path/to/podman-api.sock tests/podman-isolation/test.sh
~~~

## 变更依赖的检查清单

- 先更新 deps/sources.lock。
- 检查 build-deps workflow 在 macOS、Linux amd64、Linux arm64 全部成功。
- 确认 Alpine rootfs 的 apk 包和 guest agent 启动流程一致。
- 更新 deps.lock 的 release 版本和 SHA-256。
- 再完成应用构建和 VM smoke test。
