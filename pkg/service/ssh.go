package service

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	ssh2 "linuxvm/pkg/ssh_v2"
	"linuxvm/pkg/vmconfig"
	"path/filepath"
	"time"

	"al.essio.dev/pkg/shellescape"
	"github.com/sirupsen/logrus"
)

func AttachGuestConsole(ctx context.Context, rootfsPath string, enablePTY bool, cmdline ...string) error {
	var (
		sshClient *ssh2.Client
		err       error
	)
	if rootfsPath == "" {
		return fmt.Errorf("no rootfs specified, please provide the rootfs path")
	}

	rootfsPath, err = filepath.Abs(filepath.Clean(rootfsPath))
	if err != nil {
		return err
	}

	// ExtractToDir command line arguments
	if len(cmdline) == 0 {
		cmdline = []string{filepath.Join("/", "bin", "sh")}
	}
	logrus.Infof("run cmdline: %v", cmdline)

	// Load VM configuration
	vmc, err := vmconfig.LoadVMCFromFile(filepath.Join(rootfsPath, define.VMConfigFilePathInGuest))
	if err != nil {
		return err
	}

	sshClient, err = MakeSSHClient(ctx, vmc)
	if err != nil {
		return err
	}

	defer sshClient.Close()

	if enablePTY {
		return sshClient.Shell(ctx)
	}

	return sshClient.Run(ctx, shellescape.QuoteCommand(cmdline))
}

func MakeSSHClient(ctx context.Context, vmc *vmconfig.VMConfig) (*ssh2.Client, error) {
	var (
		client    *ssh2.Client
		err       error
		gvCtlAddr *network.Addr
	)
	if vmc.TSI {
		guestSSHAddr := fmt.Sprintf("%s:%d", define.LocalHost, define.GuestSSHServerPort)
		client, err = ssh2.Dial(ctx, guestSSHAddr,
			ssh2.WithUser(define.DefaultGuestUser),
			ssh2.WithPrivateKey(vmc.SSHInfo.HostSSHPrivateKeyFile),
			ssh2.WithTimeout(2*time.Second),
			ssh2.WithKeepalive(2*time.Second))
		if err != nil {
			return nil, err
		}
	} else {
		gvCtlAddr, err = network.ParseUnixAddr(vmc.GVPCtlAddr)
		if err != nil {
			return nil, err
		}
		guestAddr := fmt.Sprintf("%s:%d", define.GuestIP, define.GuestSSHServerPort)
		client, err = ssh2.Dial(ctx, guestAddr,
			ssh2.WithUser(define.DefaultGuestUser),
			ssh2.WithPrivateKey(vmc.SSHInfo.HostSSHPrivateKeyFile),
			ssh2.WithTimeout(2*time.Second),
			ssh2.WithKeepalive(2*time.Second),
			ssh2.WithTunnel(gvCtlAddr.Path),
		)
		if err != nil {
			return nil, err
		}
	}

	return client, nil
}
