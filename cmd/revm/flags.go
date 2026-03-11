package main

import (
	"linuxvm/pkg/define"

	"github.com/urfave/cli/v3"
)

// Common flags shared between chroot and docker commands
var (
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
		Usage: "attach an ext4 raw disk image to the VM (format: <path>[,uuid]); auto-created if the file does not exist; UUID is auto-generated when omitted; mounted at /mnt/<UUID> inside the guest; can be specified multiple times",
	}

	mountFlag = &cli.StringSliceFlag{
		Name:  define.FlagMount,
		Usage: "share a host directory into the guest via VirtIO-FS (format: /host/path:/guest/path[,ro]); can be specified multiple times",
	}

	proxyFlag = &cli.BoolFlag{
		Name:  define.FlagUsingSystemProxy,
		Usage: "read the macOS system HTTP/HTTPS proxy and forward it to the guest as http_proxy/https_proxy env vars; in gvisor mode, 127.0.0.1 is automatically rewritten to host.containers.internal",
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
		Usage: "session name; used to derive the workspace directory (/tmp/<session_id>); defaults to a random string; sessions with the same name are mutually exclusive via flock",
	}

	manageAPIFlag = &cli.StringFlag{
		Name:  define.FlagManageAPIFile,
		Usage: "custom Unix socket path for the host-side VM management API; defaults to /tmp/<session_id>/socks/vmctl.sock",
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
