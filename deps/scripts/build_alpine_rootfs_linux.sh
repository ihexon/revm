#!/usr/bin/env bash
set -xe
set -o pipefail

# shellcheck source=common.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

PKG_NAME="alpine-rootfs"
WORKSPACE="$DEPS_WORK_DIR"
ROOTFS="$WORKSPACE/$PKG_NAME"
CONTAINER="$PKG_NAME-$ARCH"
RELEASE_TAR="$DEPS_DIST_DIR/$PKG_NAME-$PLT-$ARCH.tar.zst"

cleanup() {
    docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
}

build_alpine_rootfs_linux() {
    cd "$WORKSPACE"
    cleanup
    rm -rf "$ROOTFS"
    mkdir -p "$ROOTFS"

	docker run --name="$CONTAINER" "alpine:$ALPINE_VERSION" \
		sh -c "apk add --no-cache bash ca-certificates dropbear iproute2 nftables openntpd podman tar util-linux zstd && rm -rf /var/lib/containers"

	docker export "$CONTAINER" | tar -x -C "$ROOTFS"
	install -D -m 0644 "$DEPS_DIR/config/containers.conf" "$ROOTFS/etc/containers/containers.conf"
	# libkrun's passthrough virtio-fs backend uses this xattr to present
	# guest ownership independently from the host UID/GID. Bake it into the
	# rootfs archive so runtime VM creation does not need a recursive walk.
	python3 - "$ROOTFS" <<'PY'
import os
import stat
import sys

root = sys.argv[1]
for current, dirs, files in os.walk(root, topdown=True, followlinks=False):
    entries = [current]
    entries.extend(os.path.join(current, name) for name in dirs)
    entries.extend(os.path.join(current, name) for name in files)
    for path in entries:
        try:
            info = os.lstat(path)
            if stat.S_ISLNK(info.st_mode):
                continue
            value = f"0:0:0{stat.S_IMODE(info.st_mode):o}".encode()
            mode = stat.S_IMODE(info.st_mode)
            writable = mode | 0o200
            if writable != mode:
                os.chmod(path, writable)
            try:
                os.setxattr(path, "user.containers.override_stat", value, follow_symlinks=False)
            finally:
                if writable != mode:
                    os.chmod(path, mode)
        except OSError as exc:
            raise SystemExit(f"set virtiofs ownership xattr on {path}: {exc}")
PY
}

release() {
    cd "$WORKSPACE"
	tar --xattrs --xattrs-include='user.containers.override_stat' --zstd -cvf "$RELEASE_TAR" -C "$ROOTFS" .
}

trap cleanup EXIT

build_alpine_rootfs_linux
release
