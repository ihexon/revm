<h1 align="center"><b>revm</b></h1>
<p align="center">revm helps you quickly launch Linux VMs / Containers</p>

[![build.yml](https://github.com/ihexon/revm/actions/workflows/build.yml/badge.svg)](https://github.com/ihexon/revm/actions/workflows/build.yml)

> [!WARNING]
> This project is currently under heavy development

[README_EN](./README.md) | [README_ZH](./README_zh.md)

A lightweight Linux microVM for macOS powered by [libkrun](https://github.com/containers/libkrun). Two independent
modes: **chroot mode** (run commands inside an isolated Linux rootfs) and **docker mode** (run a full Podman container
engine on Apple Silicon).

## Requirements

macOS Sonoma or later

## Installation

```bash
wget https://github.com/ihexon/revm/releases/download/<TAG>/revm-Darwin-arm64.tar.zst

xattr -d com.apple.quarantine revm-Darwin-arm64.tar.zst

tar -xvf revm-Darwin-arm64.tar.zst
```

---

## Documentation

| Document | Description |
|----------|-------------|
| [chroot mode](docs/chroot-mode.md) | Linux chroot alternative on macOS — run any rootfs with near-native performance |
| [docker mode](docs/docker-mode.md) | Full container engine without Docker Desktop — Podman/Docker CLI compatible |
| [attach](docs/attach.md) | Connect to a running VM instance |
| [workspace & networking](docs/insider.md) | Session workspace layout, reuse/cleanup, and network backends (gvisor / tsi) |
| [management API](docs/management-api.md) | VM management API via Unix socket |

## Bug Reports

https://github.com/ihexon/revm/issues

## License

Apache License 2.0 — see [LICENSE](./LICENSE) for details.

> Some parts of this document were written using AI assistance because I was lazy.
