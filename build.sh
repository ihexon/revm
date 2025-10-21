#! /usr/bin/env bash
set -e

# ============================================================================
# Constants and Configuration
# ============================================================================
readonly RED="\033[31m"
readonly YELLOW="\033[33m"
readonly GREEN="\033[32m"
readonly RESET="\033[0m"

readonly ROOTFS_LINUX_ARM64_URL="https://github.com/ihexon/revm-assets/releases/download/v1.0/rootfs-linux-arm64.tar.zst"
readonly LIBKRUN_DARWIN_ARM64_URL="https://github.com/ihexon/revm-assets/releases/download/v1.0.1/libkrun-darwin-arm64.tar.zst"
readonly LIBEXEC_DARWIN_ARM64_URL="https://github.com/ihexon/revm-assets/releases/download/v1.0/libexec-darwin-arm64.tar.zst"
readonly MIN_MACOS_VERSION="13.1"

# ============================================================================
# Logging Functions
# ============================================================================
log_err() {
	echo -e "${RED}[ERROR]${RESET} $*" >&2
	exit 100
}

log_warn() {
	echo -e "${YELLOW}[WARN]${RESET} $*" >&2
}

log_info() {
	echo -e "${GREEN}[INFO]${RESET} $*"
}

# ============================================================================
# Platform Detection
# ============================================================================
# Detects and exports platform and architecture variables
# Exports:
#   - PLT: Platform (darwin, linux)
#   - ARCH: Architecture (arm64, amd64)
detect_platform_arch() {
	local uname_s uname_m
	uname_s="$(uname)"
	uname_m="$(uname -m)"

	case "${uname_s}" in
		Darwin)
			export PLT="darwin"
			;;
		Linux)
			export PLT="linux"
			;;
		*)
			log_err "Unsupported OS: ${uname_s}"
			;;
	esac

	case "${uname_m}" in
		arm64 | aarch64)
			export ARCH="arm64"
			;;
		x86_64 | X86_64 | amd64 | AMD64)
			export ARCH="amd64"
			;;
		*)
			log_err "Unsupported architecture: ${uname_m}"
			;;
	esac

	log_info "Detected platform: ${PLT}, architecture: ${ARCH}"
}

# ============================================================================
# Utility Functions
# ============================================================================
usage() {
	cat << EOF
Usage: $(basename "$0") <action>

Actions:
  test           Run tests
  build_linux    Build binaries for Linux (Not yet implemented)
  build_darwin   Build binaries for macOS (Darwin)
EOF
	exit 1
}

# Ensures directory exists, creating it if necessary
ensure_dir() {
	local dir="$1"
	if [[ ! -d "$dir" ]]; then
		log_info "Creating directory: $dir"
		mkdir -p "$dir"
	fi
}

# Changes to directory with error handling
safe_cd() {
	local dir="$1"
	cd "$dir" || log_err "Failed to change to directory: $dir"
}

# Checks if required command exists
require_command() {
	local cmd="$1"
	if ! command -v "$cmd" &> /dev/null; then
		log_err "Required command not found: $cmd"
	fi
}

# ============================================================================
# Initialization
# ============================================================================
# Initializes workspace and environment variables
# Exports:
#   - WORKSPACE: Absolute path of build.sh directory
#   - OUTDIR: Output directory path
#   - GIT_COMMIT_ID: Current git commit short hash
#   - GIT_TAG: Current git tag
init_workspace() {
	WORKSPACE="$(realpath "$(dirname "$0")")"
	export WORKSPACE
	export OUTDIR="$WORKSPACE/out"

	log_info "Workspace: $WORKSPACE"
	safe_cd "$WORKSPACE"

	detect_platform_arch

	GIT_COMMIT_ID="$(git rev-parse --short HEAD 2> /dev/null || echo "unknown")"
	GIT_TAG="$(git describe --tags --abbrev=0 2> /dev/null || echo "unknown")"
	export GIT_COMMIT_ID GIT_TAG

	log_info "Version: ${GIT_TAG}, Commit: ${GIT_COMMIT_ID}"
}

# Validates required dependencies
check_dependencies() {
	local deps=("git" "go" "zstd" "tar" "wget")

	case "$PLT" in
		darwin)
			deps+=("codesign" "install_name_tool")
			require_command "git-lfs"
			;;
		linux) ;;
	esac

	for cmd in "${deps[@]}"; do
		require_command "$cmd"
	done

	log_info "All required dependencies are available"
}

# ============================================================================
# Download and Setup Functions
# ============================================================================
# Download libkrun library files to output directory
download_libkrun() {
	local tarball="/tmp/$(basename "$LIBKRUN_DARWIN_ARM64_URL")"
	local dest="$OUTDIR/lib"
	ensure_dir $dest

	if ! wget -c -q --show-progress --output-document="$tarball" "$LIBKRUN_DARWIN_ARM64_URL"; then
		log_err "Failed to download libkrun from $LIBKRUN_DARWIN_ARM64_URL"
	fi

	if ! tar --strip-components=1 -xf "$tarball" -C "$dest"; then
		log_err "Failed to extract libkrun"
	fi
	log_info "$tarball downloaded and extracted successfully"

}

# Copies Darwin-specific tools to output directory
download_darwin_tools() {
	local tarball="/tmp/$(basename "$LIBEXEC_DARWIN_ARM64_URL")"
	local dest="$OUTDIR/libexec"
	ensure_dir $dest

	if ! wget -c -q --show-progress --output-document="$tarball" "$LIBEXEC_DARWIN_ARM64_URL"; then
		log_err "Failed to download libexec from $LIBEXEC_DARWIN_ARM64_URL"
	fi

	if ! tar --strip-components=1 -xf "$tarball" -C "$dest"; then
		log_err "Failed to extract libexec"
	fi
	log_info "$tarball downloaded and extracted successfully"

}

# Downloads and extracts the built-in rootfs
download_builtin_rootfs() {
	local tarball="/tmp/$(basename "$ROOTFS_LINUX_ARM64_URL")"
	local dest="$OUTDIR/rootfs"
	ensure_dir "$dest"

	if ! wget -c -q --show-progress --output-document="$tarball" "$ROOTFS_LINUX_ARM64_URL"; then
		log_err "Failed to download rootfs from $ROOTFS_LINUX_ARM64_URL"
	fi

	if ! tar --strip-components=1 -xf "$tarball" -C "$dest"; then
		log_err "Failed to extract rootfs"
	fi

	log_info "$tarball downloaded and extracted successfully"
}

# Downloads all third-party dependencies based on platform
download_dependencies() {
	log_info "Downloading dependencies for platform: $PLT"

	case "$PLT" in
		darwin)
			download_libkrun
			download_darwin_tools
			download_builtin_rootfs
			;;
		linux)
			log_warn "Linux dependency download not yet implemented"
			;;
		*)
			log_err "Unsupported platform: ${PLT}"
			;;
	esac

	log_info "Dependencies downloaded successfully"
}

# ============================================================================
# Build Functions
# ============================================================================
# Builds the guest agent binary directly
# Guest agent always targets Linux ARM64 as it runs inside the VM
build_guest_agent() {
	local src_dir="$WORKSPACE/guest-agent"
	local output_dir="$OUTDIR/libexec"
	local output_binary="$output_dir/guest-agent"

	# Guest agent is always built for Linux ARM64 (runs inside VM)
	local target_os="linux"
	local target_arch="arm64"

	if [[ ! -d "$src_dir" ]]; then
		log_err "Guest agent source directory not found: $src_dir"
	fi

	if [[ ! -f "$src_dir/main.go" ]]; then
		log_err "Guest agent main.go not found: $src_dir/main.go"
	fi

	ensure_dir "$output_dir"

	log_info "Building guest agent (${target_os}/${target_arch})"

	# First change to guest-agent source dir
	safe_cd $src_dir

	if ! GOOS="$target_os" GOARCH="$target_arch" \
		go build \
		-ldflags="-s -w" \
		-o "$output_binary" \
		"$src_dir/main.go"; then
		log_err "Failed to build guest agent"
	fi

	safe_cd $WORKSPACE

	log_info "Guest agent built successfully: $output_binary"
}

# Builds the main revm binary for the target platform
build_revm() {
	local bin_dir="$OUTDIR/bin"
	ensure_dir "$bin_dir"

	local revm_bin="$bin_dir/revm"
	rm -f "$revm_bin"

	local ldflags="-X linuxvm/pkg/define.Version=$GIT_TAG -X linuxvm/pkg/define.CommitID=$GIT_COMMIT_ID"

	log_info "Building revm for ${PLT}/${ARCH}"

	local build_env=(
		"GOOS=$PLT"
		"GOARCH=$ARCH"
	)

	# Add macOS-specific build flags
	if [[ "$PLT" == "darwin" ]]; then
		build_env+=(
			"CGO_CFLAGS=-mmacosx-version-min=${MIN_MACOS_VERSION}"
			"CGO_LDFLAGS=-mmacosx-version-min=${MIN_MACOS_VERSION}"
		)
		ldflags="-extldflags=-mmacosx-version-min=${MIN_MACOS_VERSION} ${ldflags}"
	fi

	# Build the binary
	if ! env "${build_env[@]}" go build -ldflags="$ldflags" -v -o "$revm_bin" ./cmd; then
		log_err "Failed to build revm binary"
	fi

	# Post-build processing for macOS
	if [[ "$PLT" == "darwin" ]]; then
		sign_and_configure_macos_binary "$revm_bin"
	fi

	log_info "revm binary built successfully: $revm_bin"
}

# Signs and configures macOS binary with entitlements and rpath
sign_and_configure_macos_binary() {
	local binary="$1"
	local rpath="@executable_path/../lib/"
	local entitlements="$WORKSPACE/revm.entitlements"

	if [[ ! -f "$binary" ]]; then
		log_err "Binary not found: $binary"
	fi

	log_info "Codesigning binary (initial)"
	if ! codesign --force --deep --sign - "$binary"; then
		log_err "Failed to codesign binary"
	fi

	log_info "Adding rpath: $rpath"
	if ! install_name_tool -add_rpath "$rpath" "$binary" 2> /dev/null; then
		log_warn "Failed to add rpath (may already exist)"
	fi

	if [[ -f "$entitlements" ]]; then
		log_info "Codesigning with entitlements"
		if ! codesign --entitlements "$entitlements" --force -s - "$binary"; then
			log_err "Failed to codesign with entitlements"
		fi
	else
		log_warn "Entitlements file not found: $entitlements"
	fi
}

# ============================================================================
# Packaging Functions
# ============================================================================
# Creates a compressed archive of the build output
create_package() {
	local archive_name="revm-${PLT}-${ARCH}-${GIT_TAG}.tar.zst"
	local output_path="$WORKSPACE/$archive_name"

	require_command "tar"

	log_info "Creating package: $archive_name"

	if ! tar --zst -cf "$output_path" -C "$WORKSPACE" "$(basename "$OUTDIR")"; then
		log_err "Failed to create package"
	fi

	local size
	size=$(du -h "$output_path" | cut -f1)
	log_info "Package created successfully: $output_path (${size})"
}

# ============================================================================
# Build Orchestration
# ============================================================================
# Runs the complete build process for Darwin
build_darwin() {
	log_info "Starting Darwin build process"

	init_workspace
	check_dependencies
	download_dependencies
	build_revm
	build_guest_agent
	create_package

	log_info "Darwin build completed successfully"
}

# Runs the complete build process for Linux
build_linux() {
	log_err "Linux build is not yet implemented"
}

# Runs test suite
run_tests() {
	log_info "Running tests"

	if ! go test -v linuxvm/test/system; then
		log_err "Tests failed"
	fi

	log_info "All tests passed"
}

# ============================================================================
# Main Entry Point
# ============================================================================
main() {
	local action="${1:-}"

	if [[ -z "${action}" ]]; then
		usage
	fi

	case "${action}" in
		test)
			run_tests
			;;
		build_darwin)
			build_darwin
			;;
		build_linux)
			build_linux
			;;
		*)
			log_err "Unknown action: ${action}"
			usage
			;;
	esac
}

# Run main function with all arguments
main "$@"
