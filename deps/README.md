# Dependency Builds

This directory owns the dependency assets used by `revm` release builds.

The dependency build is intentionally separate from the normal `revm` build:

- `deps/sources.lock` pins the upstream source inputs used to build dependency
  assets.
- `.github/workflows/build-deps.yml` builds those assets on demand.
- `scripts/build.go` pins the released dependency assets that it downloads and
  verifies.

Run the dependency workflow manually when one of the pinned sources, build
scripts, patches, or configs changes. The workflow publishes a `deps-*` GitHub
release under this repository.

Normal `revm` builds do not rebuild these dependencies. They consume the
archives pinned by `scripts/build.go` so application builds stay fast and
repeatable.

## Layout

```text
deps/
  config/       libkrunfw configs and rootfs config files
  docker/       container images used by Linux dependency builds
  patches/      source patches applied during dependency builds
  scripts/      platform-specific dependency build scripts
  sources.lock  pinned source versions and commit IDs
```

Generated files are written under `deps/.work/` and `deps/dist/`; both are
ignored by git.
