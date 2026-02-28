# Single Binary Distribution

`revm-single` is a self-contained single binary that embeds revm and all of its runtime dependencies (shared libraries,
helper binaries, rootfs). No installation step required — download one file and run it.

## How It Works

The build process packs the entire revm install tree (`bin/`, `lib/`, `helper/`, etc.) into a `payload.tar` archive
that is embedded in the Go binary at compile time via `//go:embed`.

On first run, `revm-single`:

1. Computes a cache directory at `/tmp/.revm-<hash>` where `<hash>` is the first 16 characters of the SHA-256 of the
   `revm` binary (set at build time as `buildID`).
2. Checks if the cache already exists (i.e. `bin/revm` is present). If so, skips extraction.
3. Otherwise, extracts `payload.tar` into a temporary directory, then atomically renames it to the cache path.
4. Execs the real `revm` binary from the cache directory, forwarding all CLI arguments.

On **Linux ARM64**, the launcher additionally invokes the embedded `ld-linux-aarch64.so.1` dynamic linker with
`--library-path` pointing to the extracted `lib/` directory, so no system-level shared libraries are needed.

Subsequent runs are instant — the payload is only extracted once per build.

## Download & Usage

```bash
# Download the latest release
wget https://github.com/ihexon/revm/releases/download/<TAG>/revm-single-<OS>-<ARCH>.tar.zst

# Remove macOS quarantine attribute (macOS only)
xattr -d com.apple.quarantine revm-single-*.tar.zst

# Extract
tar -xvf revm-single-*.tar.zst

# Run — all revm subcommands work as normal
./revm-single chroot -- uname -r
./revm-single docker --workspace ~/revm_workspace
```

## Building from Source

Prerequisites: Go toolchain, `bsdtar`, and a pre-built revm install directory.

```bash
# Build revm first (produces output in ./out)
# Then build the single binary:
./cmd/single-binary/build.sh --revm-install-dir ./out
```

The script:

1. Archives the install directory into `cmd/single-binary/payload.tar`
2. Computes a `buildID` from the SHA-256 of `bin/revm`
3. Builds a static Go binary with `CGO_ENABLED=0` and embeds the payload
4. Signs the binary with ad-hoc codesign on macOS
5. Packages the result as `revm-single-<OS>-<ARCH>.tar.zst`

## Reference

- Launcher source: [`cmd/single-binary/main.go`](../cmd/single-binary/main.go)
- Build script: [`cmd/single-binary/build.sh`](../cmd/single-binary/build.sh)
