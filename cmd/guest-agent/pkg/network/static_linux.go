//go:build linux && (arm64 || amd64)

package network

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"net"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

// ConfigureStaticIPv4 configures the fixed point-to-point network used by
// gvproxy. Its address plan is explicit, so guest startup does not depend on
// a raw packet socket or a DHCP broadcast exchange.
func ConfigureStaticIPv4(ctx context.Context, ifName, address, gateway, nameserver string) error {
	logrus.Infof("configuring static IPv4 on %s: address=%s gateway=%s dns=%s", ifName, address, gateway, nameserver)
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
	logrus.Infof("static IPv4 network ready on %s", ifName)
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
