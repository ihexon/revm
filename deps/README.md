# 依赖构建

[English version](README.en.md)

revm 的 libkrun、libkrunfw 和 Alpine rootfs 不在普通应用构建中本地编译。它们由 GitHub Actions 构建为依赖 release，应用构建通过 deps.lock 下载并校验固定资产。

## 两层构建

1. 依赖构建：手动运行 .github/workflows/build-deps.yml。
2. 应用构建：运行 go run ./scripts --build revm，消费 deps.lock 指向的 release。

这样可以把需要特定 runner、Docker、Rust、交叉架构和虚拟化依赖的构建从日常 Go 开发中隔离出来。

## 依赖来源

deps/sources.lock 固定上游源：

- libkrun main
- libkrunfw
- gvisor-tap-vsock
- Alpine 版本和构建输入
- libarchive 及其他原生依赖

deps.lock 固定已经发布的资产名称、版本和 SHA-256。应用构建下载后会校验 checksum；资产缺失或校验不匹配会直接失败。

## 依赖 workflow

在 GitHub Actions 中手动触发 build-deps，并提供形如 deps-v20260928.1 的 release tag。workflow 会：

- 在 Linux amd64 和 arm64 runner 上构建 Alpine rootfs、libkrun 和 libkrunfw；
- 在 macOS arm64 runner 上构建 libkrun、libkrunfw 及其图形/渲染依赖；
- 将 Alpine rootfs 中预装 bash、ca-certificates、dropbear、iproute2、nftables、openntpd、podman、tar、util-linux 和 zstd；
- 为 rootfs 和 VirtIO-FS 目录写入 user.containers.override_stat 元数据；
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

它会下载当前平台的依赖资产，嵌入 guest-agent 和静态资源，链接 revm，并在 out/revm 生成 bundle。这个流程不会在本机重新编译 libkrun 或 libkrunfw。

## 变更依赖的检查清单

- 先更新 deps/sources.lock。
- 检查 build-deps workflow 在 macOS、Linux amd64、Linux arm64 全部成功。
- 确认 Alpine rootfs 的 apk 包和 guest agent 启动流程一致。
- 更新 deps.lock 的 release 版本和 SHA-256。
- 再运行 Go 单元测试、应用构建和 VM smoke test。
