package service

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	ssh2 "linuxvm/pkg/ssh_v2"
	"time"
)

func MakeSSHClient(ctx context.Context, vmc *define.VMConfig) (*ssh2.Client, error) {
	// Build SSH dial options based on network mode
	dialOpts := []ssh2.Option{
		ssh2.WithUser(define.DefaultGuestUser),
		ssh2.WithPrivateKey(vmc.SSHInfo.HostSSHPrivateKeyFile),
		ssh2.WithTimeout(2 * time.Second),
		ssh2.WithKeepalive(2 * time.Second),
	}

	var guestAddr string
	if vmc.VirtualNetworkMode == define.GVISOR.String() {
		// GVISOR mode: use tunnel through gvproxy
		gvCtlAddr, err := network.ParseUnixAddr(vmc.GVPCtlAddr)
		if err != nil {
			return nil, err
		}
		guestAddr = fmt.Sprintf("%s:%d", define.GuestIP, define.GuestSSHServerPort)
		dialOpts = append(dialOpts, ssh2.WithTunnel(gvCtlAddr.Path))
	} else {
		// TSI mode: direct TCP connection
		guestAddr = fmt.Sprintf("%s:%d", define.LocalHost, define.GuestSSHServerPort)
	}

	return ssh2.Dial(ctx, guestAddr, dialOpts...)
}
