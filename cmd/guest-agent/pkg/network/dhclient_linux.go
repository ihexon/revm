//go:build linux && (arm64 || amd64)

package network

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"net"
	"os"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/nclient4"
	"github.com/insomniacslk/dhcp/netboot"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

func DHClient4(ctx context.Context, ifName string, attempts int) error {
	logrus.Infof("configuring DHCPv4 on %s", ifName)
	// Wait for the interface to appear — the VMM may not have created
	// the virtual NIC by the time the guest-agent starts.
	ife, err := waitInterfaceUp(ctx, ifName)
	if err != nil {
		return fmt.Errorf("failed to bring interface %q up: %w", ifName, err)
	}

	bootConf, err := dhclient4(ctx, ife, attempts)
	if err != nil {
		return fmt.Errorf("failed to get netboot bootConf from DHCPv4 server: %w", err)
	}

	if err = netboot.ConfigureInterface(ifName, &bootConf.NetConf); err != nil {
		return fmt.Errorf("failed to apply netboot configuration: %w", err)
	}

	return nil
}

// ConfigureStaticIPv4 configures the fixed point-to-point network used by
// gvproxy. Its address plan is explicit, so guest startup does not depend on
// a raw packet socket or a DHCP broadcast exchange.
func ConfigureStaticIPv4(ctx context.Context, ifName, address, gateway, nameserver string) error {
	if _, err := waitInterfaceUp(ctx, ifName); err != nil {
		return err
	}

	link, err := netlink.LinkByName(ifName)
	if err != nil {
		return fmt.Errorf("cannot find interface %q: %w", ifName, err)
	}
	ip, network, err := net.ParseCIDR(address)
	if err != nil {
		return fmt.Errorf("parse static address %q: %w", address, err)
	}
	network.IP = ip
	if err := netlink.AddrReplace(link, &netlink.Addr{IPNet: network}); err != nil {
		return fmt.Errorf("configure static address %q on %s: %w", address, ifName, err)
	}

	gw := net.ParseIP(gateway).To4()
	if gw == nil {
		return fmt.Errorf("parse gateway %q", gateway)
	}
	if err := netlink.RouteReplace(&netlink.Route{
		LinkIndex: link.Attrs().Index,
		Dst:       &net.IPNet{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)},
		Gw:        gw,
		Src:       ip.To4(),
	}); err != nil {
		return fmt.Errorf("configure default route via %s: %w", gateway, err)
	}
	if err := os.WriteFile("/etc/resolv.conf", []byte("nameserver "+nameserver+"\n"), 0644); err != nil {
		return fmt.Errorf("write resolv.conf: %w", err)
	}
	return nil
}

func waitInterfaceUp(ctx context.Context, ifName string) (*net.Interface, error) {
	ticker := time.NewTicker(define.DefaultTimeTicker)
	defer ticker.Stop()

	for {
		ife, err := bringInterfaceUpFast(ifName)
		if err == nil {
			return ife, nil
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("waiting for interface %q: %w", ifName, ctx.Err())
		case <-ticker.C:
		}
	}
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

	client, err = nclient4.New(iface.Name, nclient4.WithRetry(1), nclient4.WithTimeout(time.Second))
	if err != nil {
		return nil, err
	}
	defer client.Close()
	logrus.Infof("DHCPv4 client ready on %s", iface.Name)

	ticker := time.NewTicker(define.DefaultTimeTicker)
	defer ticker.Stop()

	for {
		if attempts == 0 {
			return nil, fmt.Errorf("failed to obtain DHCP lease for %q after %d attempts: %w", iface.Name, maxAttempts, err)
		}
		attempts--
		logrus.Infof("DHCPv4 request on %s (attempt %d/%d)", iface.Name, maxAttempts-attempts, maxAttempts)

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
