# 单文件分发

`revm-single` 是一个自包含的单文件可执行程序，内嵌了 revm 及其所有运行时依赖（共享库、辅助工具、rootfs）。无需安装步骤——下载一个文件即可直接运行。

## 工作原理

构建过程将完整的 revm 安装目录（`bin/`、`lib/`、`helper/` 等）打包为 `payload.tar` 归档文件，并在编译时通过 Go 的 `//go:embed` 嵌入到二进制文件中。

首次运行时，`revm-single` 会：

1. 根据 `buildID`（构建时通过 `revm` 二进制的 SHA-256 前 16 位生成）计算缓存目录 `/tmp/.revm-<hash>`。
2. 检查缓存是否已存在（即 `bin/revm` 是否存在）。如果存在，跳过解压。
3. 否则，将 `payload.tar` 解压到临时目录，然后原子性地重命名为缓存路径。
4. 执行缓存目录中的 `revm` 二进制文件，转发所有命令行参数。

在 **Linux ARM64** 上，启动器还会调用内嵌的 `ld-linux-aarch64.so.1` 动态链接器，并通过 `--library-path` 指向解压后的 `lib/` 目录，因此不需要系统级的共享库。

后续运行是即时的——每次构建只需解压一次 payload。

## 下载与使用

```bash
# 下载最新版本
wget https://github.com/ihexon/revm/releases/download/<TAG>/revm-single-<OS>-<ARCH>.tar.zst

# 移除 macOS 隔离属性（仅 macOS）
xattr -d com.apple.quarantine revm-single-*.tar.zst

# 解压
tar -xvf revm-single-*.tar.zst

# 运行——所有 revm 子命令正常使用
./revm-single chroot -- uname -r
./revm-single docker --workspace ~/revm_workspace
```

## 从源码构建

前置条件：Go 工具链、`bsdtar`，以及预先构建好的 revm 安装目录。

```bash
# 先构建 revm（输出到 ./out）
# 然后构建单文件二进制：
./cmd/single-binary/build.sh --revm-install-dir ./out
```

该脚本执行以下步骤：

1. 将安装目录打包为 `cmd/single-binary/payload.tar`
2. 通过 `bin/revm` 的 SHA-256 计算 `buildID`
3. 使用 `CGO_ENABLED=0` 构建静态 Go 二进制并嵌入 payload
4. 在 macOS 上使用 ad-hoc codesign 签名
5. 将结果打包为 `revm-single-<OS>-<ARCH>.tar.zst`

## 参考

- 启动器源码：[`cmd/single-binary/main.go`](../cmd/single-binary/main.go)
- 构建脚本：[`cmd/single-binary/build.sh`](../cmd/single-binary/build.sh)
