# revm

`revm` is a single CLI for running and controlling lightweight Linux microVM sessions.

- `revm run`: runs isolated Linux command environments.
- `revm dockerd`: runs isolated Linux container environments with Docker CLI / Podman CLI compatibility.
- `revm ctl`: controls existing sessions, including attach and port forwarding updates.

The CLI has one public entrypoint and uses explicit subcommands for each operation.

## Linux Release Portability

Linux release archives are built to run on both glibc and musl based distributions. The public entrypoints in `bin/`
are launcher scripts that start the bundled `.real` executable through the bundled glibc dynamic linker and library
set in `lib/`. Run `bin/revm` directly after extracting the archive; do not bypass the launcher by running
`bin/revm.real`.

## Guides

- [run mode](docs/run.en.md): run commands, builds, tests, and scripts in an isolated Linux environment.
- [dockerd subcommand](docs/dockerd.en.md): run an isolated container environment with Docker CLI or Podman CLI.
