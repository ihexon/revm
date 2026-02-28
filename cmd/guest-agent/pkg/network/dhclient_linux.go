//go:build linux && (arm64 || amd64)

package network

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"net"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/nclient4"
	"github.com/insomniacslk/dhcp/netboot"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

func DHClient4(ctx context.Context, ifName string, attempts int) error {
	logrus.Infof("bring interface %s up", ifName)
	ife, err := bringInterfaceUpFast(ifName)
	if err != nil {
		return err
	}

	logrus.Infof("get netboot bootConf from DHCPv4 server")
	bootConf, err := dhclient4(ctx, ife, attempts)
	if err != nil {
		return err
	}

	logrus.Infof("apply netboot configuration to %s", ifName)
	if err = netboot.ConfigureInterface(ifName, &bootConf.NetConf); err != nil {
		return err
	}

	return nil
}

func bringInterfaceUpFast(ifName string) (*net.Interface, error) {
	link, err := netlink.LinkByName(ifName)
	if err != nil {
		return nil, fmt.Errorf("cannot find interface %q: %w", ifName, err)
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return nil, fmt.Errorf("failed to set interface %q up: %w", ifName, err)
	}

	iface, err := net.InterfaceByName(ifName)
	if err != nil {
		return nil, fmt.Errorf("cannot get net.Interface for %q: %w", ifName, err)
	}
	return iface, nil
}


func dhclient4(ctx context.Context, iface *net.Interface, attempts int) (*netboot.BootConf, error) {
	var (
		err         error
		lease       *nclient4.Lease
		client      *nclient4.Client
		maxAttempts = attempts
	)

	client, err = nclient4.New(iface.Name)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	ticker := time.NewTicker(define.DefaultTimeTicker)
	defer ticker.Stop()

	for {
		if attempts == 0 {
			return nil, fmt.Errorf("failed to obtain DHCP lease for %q after %d attempts: %w", iface.Name, maxAttempts, err)
		}
		attempts--

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			lease, err = client.Request(ctx)
			if err == nil && lease != nil && lease.ACK != nil && lease.Offer != nil {
				return netboot.ConversationToNetconfv4([]*dhcpv4.DHCPv4{lease.Offer, lease.ACK})
			}
		}
	}
}
