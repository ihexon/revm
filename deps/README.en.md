# Dependency builds

[中文版](README.md)

revm does not compile libkrun, libkrunfw, or the Alpine rootfs during the normal application build. GitHub Actions produces dependency releases; the application build downloads and verifies the assets selected by deps.lock.

## Two build layers

1. Dependency build: manually run .github/workflows/build-deps.yml.
2. Application build: run go run ./scripts and consume the release selected by deps.lock.

This keeps runner-specific Docker, Rust, cross-architecture, and native library work out of the normal Go development loop.

## Dependency inputs

deps/sources.lock pins the upstream inputs:

- a pinned commit from libkrun's 2.x development line (the application uses the 2.x API)
- a pinned libkrunfw commit
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
go run ./scripts
~~~

It downloads the current platform's dependency assets, embeds guest-agent and static resources, links revm, and writes a bundle under out/revm. It does not rebuild libkrun or libkrunfw locally.

## Testing local dependencies

Dependency scripts write archives to `deps/dist`. Consume them without publishing a release:

~~~bash
go run ./scripts --deps-dir deps/dist
~~~

Both native library archives for the current platform are required. Missing files fail the build instead of falling back to released libraries. The build logs each local archive's SHA-256; the Alpine rootfs still comes from the verified release lock. Without `--deps-dir`, the build uses released assets from `deps.lock`; changing source pins alone does not change that release.

On Apple Silicon, an existing revm can run the Linux builder image from `deps/docker/linux.Dockerfile` using Podman. Run `build_libkrunfw_linux.sh` there with `DEPS_WORK_DIR` on a container disk or named volume (kernel sources need a case-sensitive filesystem), and mount this repository at the same path to export `deps/dist`. Then run `build_libkrun_darwin.sh` and `build_libkrunfw_darwin.sh` on macOS. `DEPS_BUILD_JOBS` controls kernel parallelism (default 4); use `CARGO_BUILD_JOBS` for Rust.

## Kernel configuration

The repository's `deps/config/config-libkrunfw_*` files remain the starting point. After upstream patches, Linux `olddefconfig` resolves new dependencies. `check_kernel_config.sh` compares against the normalized upstream configuration using `diffconfig`, and requires container storage, networking, namespaces, cgroups, seccomp, and vsock support. Unreviewed upstream omissions fail the build.

Intentional exclusions are GPU display and its helpers, plus x86 32-bit compatibility. ARM64 retains 4 KB pages. Dependency archives include input/final configs and diffs under `share/revm/kernel`; application bundles copy them to `share/libkrunfw/kernel` along with source revision metadata.

Test a running disposable session with `CONTAINER_HOST=unix:///path/to/podman-api.sock tests/podman-isolation/test.sh`.

## Dependency change checklist

- Update deps/sources.lock.
- Confirm build-deps succeeds on macOS, Linux amd64, and Linux arm64.
- Check that the Alpine apk set matches guest-agent startup.
- Update the dependency release and checksums in deps.lock.
- Run the application build and VM smoke tests.
