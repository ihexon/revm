#! /usr/bin/env bash
set -e

detect_platform_arch() {
	local uname_s uname_m
	uname_s="$(uname)"
	uname_m="$(uname -m)"

	case "${uname_s}" in
		Darwin) export PLT="darwin" ;;
		Linux) export PLT="linux" ;;
		*)
			echo "Unsupported OS: ${uname_s}" >&2
			exit 1
			;;
	esac

	case "${uname_m}" in
		arm64 | aarch64) export ARCH="arm64" ;;
		x86_64 | X86_64 | amd64 | AMD64) export ARCH="amd64" ;;
		*)
			echo "Unsupported architecture: ${uname_m}" >&2
			exit 1
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
}

init_func() {
	rm -rf ./out
	mkdir -p ./out/bin && mkdir -p ./out/3rd
	detect_platform_arch
}

copy_3rd() {
	cp -av ./3rd/"$PLT" ./out/3rd/"$PLT"

	echo "codesign ./out/3rd/$PLT/lib/*"
	if [[ "$PLT" == "darwin" ]]; then
		shopt -s nullglob
		for f in ./out/3rd/"$PLT"/lib/*; do
			codesign --force --deep --sign - "$f"
		done
		shopt -u nullglob
	fi
}

build_revm() {
	local bin="out/bin/revm"
	local lib_dir="3rd/$PLT/lib"

	GOOS=$PLT GOARCH=$ARCH go build -v -o "$bin" ./cmd/main.go
}

process_binaries() {
	local bin="out/bin/revm"
	local lib_dir="3rd/$PLT/lib"

	codesign --force --deep --sign - "$bin"
	install_name_tool -add_rpath "@executable_path/../$lib_dir" "$bin"
	codesign --entitlements revm.entitlements --force -s - "$bin"
}

build_bootstrap() {
	local bin="out/3rd/$PLT/bin/bootstrap"
	echo "Build bootstrap for guest"
	GOOS=linux GOARCH=$ARCH go build -v -o "$bin" ./cmd/bootstrap
}

packaging() {
	tar --zstd -cvf revm.tar out/
}

main() {
	local script_name=$0

	local action="${1:-}"
	if [[ -z "${action}" ]]; then
		usage
		exit 1
	fi

	case "${action}" in
		test)
			echo "$0: run tests..."
			go test -v linuxvm/test/system
			;;

		build_linux)
			echo "$script_name: build for linux"
			init_func
			copy_3rd
			build_revm
			build_bootstrap
			packaging
			;;
		build_darwin)
			echo "$script_name: build for darwin"
			init_func
			copy_3rd
			build_revm
			process_binaries
			build_bootstrap
			packaging
			;;
		*)
			usage
			exit 1
			;;
	esac
}

main "$@"
