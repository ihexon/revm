# revm

`revm` is a composite project that serves as the shared codebase and runtime foundation for `chroot` and `dockerd`.

- `chroot`: runs isolated Linux command environments.
- `dockerd`: runs isolated Linux container environments with Docker CLI / Podman CLI compatibility.

Each entry command keeps the design as simple as possible and follows the KISS principle.

## Linux Release Portability

Linux release archives are built to run on both glibc and musl based distributions. The public entrypoints in `bin/`
are launcher scripts that start the bundled `.real` executable through the bundled glibc dynamic linker and library
set in `lib/`. Run `bin/chroot` or `bin/dockerd` directly after extracting the archive; do not bypass the launcher by
running `bin/chroot.real` or `bin/dockerd.real`.

## Guides

- [chroot mode](docs/chroot.en.md): run commands, builds, tests, and scripts in an isolated Linux environment.
- [dockerd mode](docs/dockerd.en.md): run an isolated container environment with Docker CLI or Podman CLI.
