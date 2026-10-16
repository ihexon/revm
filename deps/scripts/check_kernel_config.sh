#!/usr/bin/env bash
set -euo pipefail

# Delegate dependency resolution and configuration differences to Linux Kconfig.
kernel="$1"
audit="$2"
arch="$3"
"$kernel/scripts/diffconfig" "$audit/upstream.config" "$audit/final.config" > "$audit/upstream.diff"
"$kernel/scripts/diffconfig" "$audit/revm.config" "$audit/final.config" > "$audit/normalization.diff"

required=(NAMESPACES UTS_NS IPC_NS USER_NS PID_NS NET_NS CGROUPS CGROUP_PIDS
    MEMCG CPUSETS CGROUP_CPUACCT CGROUP_SCHED SECCOMP SECCOMP_FILTER
    VIRTIO_BLK VIRTIO_NET VIRTIO_FS VSOCKETS VIRTIO_VSOCKETS
    EXT4_FS OVERLAY_FS TMPFS_POSIX_ACL DEVTMPFS NETFILTER BRIDGE VETH
    NF_NAT NF_CONNTRACK NF_TABLES NF_TABLES_INET NFT_CT NFT_NAT NFT_MASQ TUN)
if [[ "$arch" == aarch64 ]]; then required+=(ARM64_4K_PAGES); fi
for option in "${required[@]}"; do
    if [[ "$("$kernel/scripts/config" --file "$audit/final.config" --state "$option")" != y ]]; then
        echo "Required kernel option CONFIG_$option must be built in" >&2
        exit 1
    fi
done

# Let Kconfig remove the dependencies of our deliberate headless/64-bit-only
# choices, rather than maintaining a growing list of derived symbols.
cp "$audit/upstream.config" "$audit/upstream-headless.config"
"$kernel/scripts/config" --file "$audit/upstream-headless.config" --disable DRM_VIRTIO_GPU
if [[ "$arch" == x86_64 ]]; then
    "$kernel/scripts/config" --file "$audit/upstream-headless.config" --disable IA32_EMULATION --disable COMPAT_32BIT_TIME
    export ARCH=x86_64
else
    export ARCH=arm64
fi
make -s -C "$kernel" KCONFIG_CONFIG="$audit/upstream-headless.config" olddefconfig
"$kernel/scripts/diffconfig" "$audit/upstream-headless.config" "$audit/final.config" > "$audit/required.diff"
"$kernel/scripts/diffconfig" -m "$audit/upstream.config" "$audit/final.config" > "$audit/upstream.fragment"
while IFS= read -r option; do
    echo "Unreviewed disabled upstream kernel option: CONFIG_$option" >&2
    exit 1
done < <(awk '
    $1 ~ /^-/ && ($2 == "y" || $2 == "m") {sub(/^-/, "", $1); print $1}
    ($2 == "y" || $2 == "m") && $3 == "->" && $4 == "n" {print $1}
' "$audit/required.diff")
