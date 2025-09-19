#! /usr/bin/env bash
set -e

trace_off() {
	{ set +x; } 2> /dev/null
}

trace_on() {
	{ set -x; } 2> /dev/null
}

RED="\033[31m"
YELLOW="\033[33m"
GREEN="\033[32m"
RESET="\033[0m"

log_err() {
	echo -e "${RED}[ERROR]${RESET} $*" >&2
	exit 100
}

log_warn() {
	echo -e "${YELLOW}[WARN]${RESET} $*" >&2
}

log_std() {
	echo -e "${GREEN}[INFO]${RESET} $*"
}

# EXPORTED VARS:
# - ARCH (arm64,amd64)
# - PLT (darwin,linux)
detect_platform_arch() {
	local uname_s uname_m
	uname_s="$(uname)"
	uname_m="$(uname -m)"

	case "${uname_s}" in
		Darwin)
			export PLT="darwin"
			;;
		Linux) export PLT="linux" ;;
		*)
			log_err "Unsupported OS: ${uname_s}" >&2
			;;
	esac

	case "${uname_m}" in
		arm64 | aarch64) export ARCH="arm64" ;;
		x86_64 | X86_64 | amd64 | AMD64) export ARCH="amd64" ;;
		*)
			log_err "Unsupported architecture: ${uname_m}" >&2
			;;
	esac
}

usage() {
	cat << EOF
Usage: $(basename "$0") <action>

Actions:
  test           Run tests
  build_linux    Build binaries for Linux
  build_darwin   Build binaries for macOS (Darwin)
EOF
	exit 1
}

# EXPORTED VARS:
# - WORKSPACE (the abs path of build.sh)
# - BUILTIN_DOCKER_RUNTIME (whatever include built-in docker runtime)
init_func() {
	WORKSPACE="$(realpath "$(dirname "$0")")"
	export WORKSPACE
	if [[ -n "$DIRTY_BUILD" ]]; then
		log_warn "!!DIRTY BUILD!!"
	else
		log_warn "DELETE OUTPUT DIR"
		rm -rf "$WORKSPACE/out" && mkdir -p ./out/bin && mkdir -p ./out/3rd
	fi

	log_std "change dir to $WORKSPACE"
	detect_platform_arch
	GIT_COMMIT_ID="$(git rev-parse --short HEAD || echo "unknown")"
	GIT_TAG="$(git describe --tags --abbrev=0 || echo "unknown")"
}

_downloader() {
	local target_dir="$1"
	local item="$2"
	local do_codesign="$3"

	local _d="$target_dir/$(basename "$item")"
	log_std "download $item to ${_d}"

	if [[ -f "${_d}" ]]; then
		return
	fi
	wget "$item" -c --output-document "${_d}"
	chmod +x "${_d}"
	if [[ $do_codesign == true ]]; then
		codesign --force --deep --sign - "${_d}"
	fi
}

_download_e2fsprogs_darwin() {
	local dir="$WORKSPACE/out/3rd/$PLT/bin"
	mkdir -p "$dir"
	local urls=()
	if [[ "$PLT" == "darwin" ]]; then
		urls+=(
			"https://github.com/ihexon/prebuilds/raw/refs/heads/main/e2fsprogs/arm64/darwin/blkid"
			"https://github.com/ihexon/prebuilds/raw/refs/heads/main/e2fsprogs/arm64/darwin/mke2fs"
			"https://github.com/ihexon/prebuilds/raw/refs/heads/main/e2fsprogs/arm64/darwin/fsck.ext4"
		)
	fi

	for item in "${urls[@]}"; do
		_downloader "$dir" "$item" "true"
	done

	cd "$WORKSPACE"
}

# Only build for macOS arm64
_download_libkrun_darwin() {
	local libkrun_dir="$WORKSPACE/out/3rd/$PLT/lib"
	mkdir -p "$libkrun_dir"
	local urls=()
	if [[ "$PLT" == "darwin" ]]; then
		urls+=(
			"https://github.com/ihexon/prebuilds/raw/f0623b33cb10ac642b901ec9216b28c7df25e8c4/libkrun/arm64/darwin/libkrun.1.15.1.dylib"
			"https://github.com/ihexon/prebuilds/raw/f0623b33cb10ac642b901ec9216b28c7df25e8c4/libkrun/arm64/darwin/libkrunfw.4.dylib"
			"https://github.com/ihexon/prebuilds/raw/refs/heads/main/libkrun/arm64/darwin/libepoxy.0.dylib"
			"https://github.com/ihexon/prebuilds/raw/refs/heads/main/libkrun/arm64/darwin/libMoltenVK.dylib"
			"https://github.com/ihexon/prebuilds/raw/refs/heads/main/libkrun/arm64/darwin/libvirglrenderer.1.dylib"
		)
	fi

	for item in "${urls[@]}"; do
		_downloader "$libkrun_dir" "$item" "true"
	done

	log_std "change dir to $libkrun_dir"
	log_std "create symbol link the libkrun"
	cd "$WORKSPACE"
}

_download_busybox_linux() {
	local busybox_bindir="$WORKSPACE/out/3rd/linux/bin"
	mkdir -p "$busybox_bindir"
	local urls=(
		"https://github.com/ihexon/prebuilds/raw/refs/heads/main/busybox/arm64/linux/busybox.static"
	)

	for item in "${urls[@]}"; do
		_downloader "$busybox_bindir" "$item" "false"
	done

	log_std "create symbol link the busybox"
	cd "$busybox_bindir"
	chmod +x busybox.static
	ln -sf busybox.static busybox
	cd "$WORKSPACE"
}

_download_dropbear() {
	local dropbear_bin_dir="$WORKSPACE/out/3rd/linux/bin"
	mkdir -p "$dropbear_bin_dir"
	local urls=(
		"https://github.com/ihexon/prebuilds/raw/refs/heads/main/dropbear/arm64/linux/dropbear"
		"https://github.com/ihexon/prebuilds/raw/refs/heads/main/dropbear/arm64/linux/dropbearkey"
	)

	for item in "${urls[@]}"; do
		_downloader "$dropbear_bin_dir" "$item" "false"
	done
}

_download_podman_rootfs() {
	if [[ $DIRTY_BUILD == "true" ]]; then
		log_warn "DIRTY_BUILD set true, skip download alpine rootfs from github lfs"
		return
	fi
	local dir="$WORKSPACE/out/3rd/linux/rootfs"
	mkdir -p "$dir"
	local url="https://github.com/ihexon/prebuilds/raw/refs/heads/main/rootfs/arm64/alpine/rootfs.tar.zst"
	wget -q --output-document - "$url" | tar --strip-components=1 -xv -C "$dir"
}

download_3rd() {
	case $PLT in
		darwin)
			_download_libkrun_darwin
			_download_busybox_linux
			_download_dropbear
			_download_e2fsprogs_darwin
			_download_podman_rootfs
			;;
		*)
			log_err "Unsupported architecture: ${PLT}"
			;;
	esac
}

build_revm() {
	local revm_bin="out/bin/revm"
	rm -f "$revm_bin"
	CGO_CFLAGS="-mmacosx-version-min=13.1" CGO_LDFLAGS="-mmacosx-version-min=13.1" GOOS=$PLT GOARCH=$ARCH go build -ldflags="-extldflags=-mmacosx-version-min=13.1 -X linuxvm/pkg/define.Version=$GIT_TAG -X linuxvm/pkg/define.CommitID=$GIT_COMMIT_ID" -v -o "$revm_bin" ./cmd/
	if [[ "$PLT" == "darwin" ]]; then
		log_std "codesign to revm"
		codesign --force --deep --sign - "$revm_bin"

		log_std "add rpath to revm"
		install_name_tool -add_rpath "@executable_path/../3rd/$PLT/lib" "$revm_bin"

		log_std "codesign revm with revm.entitlements"
		codesign --entitlements revm.entitlements --force -s - "$revm_bin"
	fi
}

build_bootstrap() {
	local boostrap_bin="out/3rd/linux/bin/bootstrap"
	log_std "Build bootstrap for guest"
	rm -f "$boostrap_bin"
	CGO_ENABLED=0 GOOS=linux GOARCH=$ARCH go build -v -ldflags="-s -w" -o "$boostrap_bin" ./cmd/bootstrap
}

packaging() {
	name=revm.tar.zst
	if [[ -n $DIRTY_BUILD ]]; then
		log_warn "SKIP PACKING"
	else
		tar --zst -cvf "$name" out/
	fi
}

main() {
	local action="${1:-}"
	if [[ -z "${action}" ]]; then
		usage
	fi

	case "${action}" in
		test)
			log_std "$0: run tests..."
			go test -v linuxvm/test/system
			;;
		build_darwin)
			init_func
			download_3rd
			build_revm
			build_bootstrap
			packaging
			;;
		*)
			usage
			;;
	esac
}

main "$@"
