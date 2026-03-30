package main

import (
	"linuxvm/pkg/define"
	"linuxvm/pkg/librevm"

	"github.com/urfave/cli/v3"
)

// Reusable CLI flags shared across revm subcommands.
var (
	rootfsFlag = &cli.StringFlag{
		Name:  define.FlagRootfs,
		Usage: "path to a rootfs directory to use as the VM root filesystem; must contain /bin/sh; takes priority over the built-in rootfs",
	}

	cpuFlag = &cli.Int8Flag{
		Name:  define.FlagCPUS,
		Usage: "number of vCPU cores to assign to the VM; defaults to host CPU count if unset or less than 1",
	}

	memoryFlag = &cli.Uint64Flag{
		Name:  define.FlagMemoryInMB,
		Usage: "VM memory size in MB; minimum 512 MB; defaults to host available memory if unset or less than 512",
	}

	envsFlag = &cli.StringSliceFlag{
		Name:  define.FlagEnvs,
		Usage: "environment variables to pass to the guest process (format: KEY=VALUE); can be specified multiple times",
	}

	diskFlag = &cli.StringSliceFlag{
		Name:  define.FlagRawDisk,
		Usage: "attach an ext4 raw disk image to the VM (format: <path>[,uuid=<uuid>][,version=<string>][,mnt=<guest-path>]); auto-created if the file does not exist; new disks default to a random UUID and mount at /mnt/<UUID>; can be specified multiple times",
	}

	mountFlag = &cli.StringSliceFlag{
		Name:  define.FlagMount,
		Usage: "share a host directory into the guest via VirtIO-FS (format: /host/path:/guest/path[,ro]); can be specified multiple times",
	}

	proxyFlag = &cli.BoolFlag{
		Name:  define.FlagUsingSystemProxy,
		Usage: "read the macOS system HTTP/HTTPS proxy and forward it to the guest as http_proxy/https_proxy env vars; in gvisor mode, 127.0.0.1 is automatically rewritten to host.containers.internal",
	}

	workDirFlag = &cli.StringFlag{
		Name:  define.FlagWorkDir,
		Usage: "working directory for command execution inside the guest; the guest-agent chdirs to this path before running the command",
		Value: "/",
	}

	networkFlag = &cli.StringFlag{
		Name:  define.FlagVNetworkType,
		Usage: "virtual network stack: gvisor uses gvisor-tap-vsock (full TCP/UDP, DNS, NAT via 192.168.127.0/24); tsi uses libkrun transparent socket interception",
		Value: string(define.GVISOR),
	}

	reportEventsFlag = &cli.StringFlag{
		Name:  define.FlagReportEvents,
		Usage: "HTTP endpoint to receive VM lifecycle events (e.g. unix:///var/run/events.sock or tcp://192.168.1.252:8888)",
	}

	logLevelFlag = &cli.StringFlag{
		Name:  define.FlagLogLevel,
		Usage: "log verbosity level (trace, debug, info, warn, error, fatal, panic)",
		Value: "info",
	}

	logToFlag = &cli.StringFlag{
		Name:  define.FlagLogTo,
		Usage: "custom log file path on host; defaults to /tmp/<session_id>/logs/vm.log when unset",
	}

	sessionIDFlag = &cli.StringFlag{
		Name:  define.FlagSessionID,
		Usage: "session name; used to derive the workspace directory (/tmp/<session_id>); sessions with the same name are mutually exclusive via flock",
		Value: librevm.RandomString(),
	}

	manageAPIFlag = &cli.StringFlag{
		Name:  define.FlagManageAPIFile,
		Usage: "custom Unix socket path for the host-side VM management API; defaults to /tmp/<session_id>/socks/vmctl.sock",
	}

	containerDiskFlag = &cli.StringFlag{
		Name:  define.FlagContainerDisk,
		Usage: "persistent ext4 raw disk image for container storage (format: <path>[,version=<string>]); auto-created if missing; if the stored version xattr is missing or mismatched, the disk is recreated; defaults to a workspace-local disk with the built-in container disk version when unset",
	}

	podmanProxyAPIFileFlag = &cli.StringFlag{
		Name:  define.FlagPodmanProxyAPIFile,
		Usage: "custom Unix socket path for the host-side Podman API proxy; defaults to /tmp/<session_id>/socks/podman-api.sock",
	}

	sshKeyDirFlag = &cli.StringFlag{
		Name:  define.FlagSSHKeyDir,
		Usage: "directory to symlink the generated SSH key pair (key and key.pub) into; keys are always created inside the session directory",
	}

	sshPrivateKeyFlag = &cli.StringFlag{
		Name:  define.FlagExportSSHKeyPrivateFile,
		Usage: "file path to symlink the generated SSH private key to",
	}

	sshPublicKeyFlag = &cli.StringFlag{
		Name:  define.FlagExportSSHKeyPublicFile,
		Usage: "file path to symlink the generated SSH public key to",
	}
)
