#! /usr/bin/env bash
set -e

init_func() {
	rm -rf ./out
	mkdir -p ./out/bin && mkdir -p ./out/3rd
	if [[ "$(uname)" == "Darwin" ]]; then
		export PLT=darwin
	elif [[ "$(uname)" == "Linux" ]]; then
		export PLT=linux
	fi

	if [[ "$(uname -m)" == "arm64" ]] || [[ "$(uname -m)" == "aarch64" ]]; then
		export ARCH="arm64"
	fi

	if [[ "$(uname -m)" == "x86_64" ]] || [[ "$(uname -m)" == "X86_64" ]] || [[ "$(uname -m)" == "amd64" ]] || [[ "$(uname -m)" == "AMD64" ]]; then
		export ARCH="x86_64"
	fi
}

copy_3rd() {
	cp -av ./3rd/"$PLT" ./out/3rd/"$PLT"

	echo "codesign ./out/3rd/$PLT/lib/*"
	if [[ "$PLT" == "darwin" ]]; then
		codesign --force --deep --sign - ./out/3rd/"$PLT"/lib/*
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
	local action=$1
	local script_name=$0

	if [[ -z $action ]]; then
		echo "suppport operation: test, build_linux, build_darwin"
		exit 1
	fi

	if [[ $action == "test" ]]; then
		echo "$script_name: run test...."
		go test -v linuxvm/test/system
	fi

	if [[ $action == "build_linux" ]]; then
		echo "$script_name: build for linux"
		init_func
		copy_3rd
		build_revm
		build_bootstrap
		packaging
	fi

	if [[ $action == "build_darwin" ]]; then
		echo "$script_name: build for darwin"
		init_func
		copy_3rd
		build_revm
		process_binaries
		build_bootstrap
		packaging
	fi

}

main "$@"
