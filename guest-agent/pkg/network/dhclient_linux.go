//go:build linux && (arm64 || amd64)

package network

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/nclient4"
	"github.com/insomniacslk/dhcp/netboot"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

func DHClient4(ctx context.Context, ifName string, attempts int) error {
	ife, err := bringInterfaceUpFast(ifName)
	if err != nil {
		return fmt.Errorf("failed to bring interface %q up: %w", ifName, err)
	}

	bootConf, err := dhclient4(ctx, ife, attempts)
	if err != nil {
		return fmt.Errorf("failed to get dhcp config: %w", err)
	}

	if err := netboot.ConfigureInterface(ifName, &bootConf.NetConf); err != nil {
		return err
	}

	logrus.Infof("network configured: %s (mac: %s)", ifName, ife.HardwareAddr)
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
	if attempts < 1 {
		attempts = 3
	}
	var (
		err    error
		lease  *nclient4.Lease
		client *nclient4.Client
	)

	client, err = nclient4.New(iface.Name)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	for attempt := 0; attempt < attempts; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		lease, err = client.Request(ctx)
		if err == nil {
			if lease == nil || lease.ACK == nil || lease.Offer == nil {
				logrus.Warnf("DHCP incomplete response (attempt %d/%d)", attempt+1, attempts)
				continue
			}
			return netboot.ConversationToNetconfv4([]*dhcpv4.DHCPv4{lease.Offer, lease.ACK})
		}

		logrus.Warnf("DHCP failed (attempt %d/%d): %v", attempt+1, attempts, err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}

	return nil, fmt.Errorf("DHCP failed after %d attempts: %w", attempts, err)
}
