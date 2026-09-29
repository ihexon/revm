# Dependency builds

[中文版](README.md)

revm does not compile libkrun, libkrunfw, or the Alpine rootfs during the normal application build. GitHub Actions produces dependency releases; the application build downloads and verifies the assets selected by deps.lock.

## Two build layers

1. Dependency build: manually run .github/workflows/build-deps.yml.
2. Application build: run go run ./scripts --build revm and consume the release selected by deps.lock.

This keeps runner-specific Docker, Rust, cross-architecture, and native library work out of the normal Go development loop.

## Dependency inputs

deps/sources.lock pins the upstream inputs:

- libkrun main
- libkrunfw
- gvisor-tap-vsock
- the Alpine version and build inputs
- libarchive and other native dependencies

deps.lock pins released asset names, version, and SHA-256. The application build verifies every downloaded checksum and fails on missing or mismatched assets.

## Dependency workflow

Manually dispatch build-deps in GitHub Actions with a release tag such as deps-v20260928.1. The workflow:

- builds Alpine rootfs, libkrun, and libkrunfw on Linux amd64 and arm64 runners;
- builds libkrun and libkrunfw on a macOS arm64 runner;
- writes user.containers.override_stat metadata for the packaged rootfs;
- generates SHA256SUMS and a manifest;
- publishes a deps-* GitHub release.

Create a new dependency release when sources.lock, build scripts, patches, or upstream Head changes.

## Layout

~~~text
deps/
  config/       libkrunfw and rootfs configuration
  docker/       Linux dependency build images
  patches/      patches applied during builds
  scripts/      platform-specific dependency scripts
  sources.lock  pinned upstream versions and commits
~~~

deps/.work and deps/dist are generated directories ignored by git.

## Application build

The application build reads deps.lock:

~~~bash
go run ./scripts --build revm
~~~

It downloads the current platform's dependency assets, embeds guest-agent and static resources, links revm, and writes a bundle under out/revm. It does not rebuild libkrun or libkrunfw locally.

## Dependency change checklist

- Update deps/sources.lock.
- Confirm build-deps succeeds on macOS, Linux amd64, and Linux arm64.
- Check that the Alpine apk set matches guest-agent startup.
- Update the dependency release and checksums in deps.lock.
- Run the application build and VM smoke tests.
