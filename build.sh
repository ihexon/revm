#! /usr/bin/env bash
set -e

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
	export OUTDIR="$WORKSPACE/out"

	log_std "change workspace to $WORKSPACE"
	cd "$WORKSPACE" || {
		echo "Failed to change to workspace directory"
		exit 1
	}
	detect_platform_arch
	GIT_COMMIT_ID="$(git rev-parse --short HEAD || echo "unknown")"
	GIT_TAG="$(git describe --tags --abbrev=0 || echo "unknown")"

	if ! git lfs version > /dev/null 2>&1; then
		log_err "Git LFS is required but not installed"
	fi
}

# Only build for macOS arm64
_download_libkrun() {
	local dir="$OUTDIR/lib"
	mkdir -p "$dir"

	log_std "copy $WORKSPACE/libkrun/* to $dir"
	cp "$WORKSPACE"/libkrun/* "$dir"
}

_download_darwin_tools() {
	local dir="$OUTDIR/libexec"
	mkdir -p "$dir"

	log_std "copy $WORKSPACE/libexec/* to $dir"
	cp "$WORKSPACE"/libexec/* "$dir"
}

_download_builtin_rootfs() {
	local dir="$OUTDIR/rootfs"
	mkdir -p "$dir"
	local tarbar="/tmp/rootfs.tar.zst"
	local url="https://github.com/ihexon/prebuilds/raw/refs/heads/main/rootfs/arm64/alpine/rootfs.tar.zst"

	log_std "download the rootfs from $url"
	wget -c -q --output-document="$tarbar" "$url"

	log_std "extract the $tarbar to $dir"
	tar --strip-components=1 -xf "$tarbar" -C "$dir"
}

download_3rd() {
	case $PLT in
		darwin)
			_download_libkrun
			_download_darwin_tools
			_download_builtin_rootfs
			;;
		*)
			log_err "Unsupported architecture: ${PLT}"
			;;
	esac
}

build_revm() {
	local revm_bin="$OUTDIR/bin/revm"
	rm -f "$revm_bin"
	CGO_CFLAGS="-mmacosx-version-min=13.1" \
		CGO_LDFLAGS="-mmacosx-version-min=13.1" \
		GOOS=$PLT \
		GOARCH=$ARCH \
		go build \
		-ldflags="-extldflags=-mmacosx-version-min=13.1 -X linuxvm/pkg/define.Version=$GIT_TAG -X linuxvm/pkg/define.CommitID=$GIT_COMMIT_ID" \
		-v -o "$revm_bin" ./cmd/

	if [[ "$PLT" == "darwin" ]]; then
		log_std "codesign to $revm_bin"
		codesign --force --deep --sign - "$revm_bin"

		local rpath_str="@executable_path/../lib/"
		log_std "add rpath $rpath_str to $revm_bin"

		install_name_tool -add_rpath "$rpath_str" "$revm_bin"

		log_std "codesign revm with revm.entitlements"
		codesign --entitlements revm.entitlements --force -s - "$revm_bin"
	fi
}

build_bootstrap() {
	local bootstrap_bin="$OUTDIR/bin/bootstrap"
	log_std "Build bootstrap for guest"
	rm -f "$bootstrap_bin"
	CGO_ENABLED=0 GOOS=linux GOARCH=$ARCH go build -v -ldflags="-s -w" -o "$bootstrap_bin" ./cmd/bootstrap
}

packaging() {
	name=revm.tar.zst
	log_std "packaging all files"
	tar --zst -cf "$name" out/
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
