# NOTICE

This file documents the dependency relationship between `revm`, `libkrun`, and
`libkrunfw`. It is an engineering notice, not a full license audit.

## Project Roles

### revm

`revm` is the Go MicroVM application in this repository.

It wraps and links against `libkrun` through cgo, embeds guest-side helper
programs, and packages host runtime libraries and static resources into release
archives.

Normal release builds consume dependency archives pinned by `scripts/build.go`.

Primary license: Apache-2.0, see `LICENSE`.

### deps

`deps/` contains the dependency build system for `revm`.

It builds and packages the runtime inputs consumed by `revm`, including:

- `libkrun`
- `libkrunfw`
- guest helper binaries such as BusyBox and Dropbear
- the root filesystem archive used by `revm`

Dependency source inputs are pinned in `deps/sources.lock`. Published
dependency assets are pinned by `scripts/build.go`.

### libkrun

`libkrun` is the upstream MicroVM/VMM library consumed by `revm`.

It provides the C API and VMM implementation that `revm` links against. The
dependency build produces `libkrun` archives for the platforms supported by
`revm`.

Primary license: Apache-2.0.

### libkrunfw

`libkrunfw` is the upstream firmware/kernel bundle library consumed by
`libkrun`.

It bundles a Linux kernel into a library format that `libkrun` can consume at
runtime. `revm` depends on `libkrunfw` through its use of `libkrun` and through
the runtime library bundle copied into release archives.

Licensing summary from the upstream project:

- bundled Linux kernel: GPL-2.0-only
- files under the `patches` directory: GPL-2.0-only
- library code and generated bundle code: LGPL-2.1-only

Binary distributions of `libkrunfw` must be accompanied by the source code for
the bundled Linux kernel and the `libkrunfw` library code. The dependency
release keeps the `libkrunfw-src-*.tar.zst` source archive for this reason.

## Build Relationship

```text
deps/sources.lock
        |
        v
deps/scripts/*
        |
        v
dependency release archives + SHA256SUMS + deps.manifest.json
        |
        v
scripts/build.go
        |
        v
revm release archive
```

In practical terms:

- `deps/` builds third-party runtime artifacts.
- `scripts/build.go` pins the exact artifacts consumed by `revm`.
- `revm` links against `libkrun` and bundles `libkrunfw` runtime libraries.
- `libkrunfw` carries additional source-distribution obligations because it
  contains a bundled Linux kernel.

This notice does not claim Debian Policy compliance. Debian packaging would
require a separate source-package workflow that builds without network access
and does not rely on prebuilt release assets.
