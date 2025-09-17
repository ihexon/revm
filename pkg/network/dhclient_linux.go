//go:build linux && (arm64 || amd64)

package network

import (
	"fmt"
	"net"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/client4"
	"github.com/insomniacslk/dhcp/netboot"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

func DHClient4(ifName string, attempts int, verbose bool) error {
	logrus.Debugf("try to bring interface %q up", ifName)
	_, err := BringInterfaceUpFast(ifName)
	if err != nil {
		return fmt.Errorf("failed to bring interface %q up: %w", ifName, err)
	}

	logrus.Debugf("get dhcp config for interface %q", ifName)
	bootConf, err := dhClient4(ifName, attempts, verbose)
	if err != nil {
		return fmt.Errorf("failed to get dhcp config: %w", err)
	}

	logrus.Debugf("configure interface %q", ifName)
	return netboot.ConfigureInterface(ifName, &bootConf.NetConf)
}

// BringInterfaceUp is slower than BringInterfaceUpFast
func BringInterfaceUp(ifName string) (_ *net.Interface, err error) {
	return netboot.IfUp(ifName, 2*time.Second)
}

func BringInterfaceUpFast(ifName string) (*net.Interface, error) {
	link, err := netlink.LinkByName(ifName)
	if err != nil {
		return nil, fmt.Errorf("cannot find interface %q: %w", ifName, err)
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return nil, fmt.Errorf("failed to set interface %q up: %w", ifName, err)
	}

	// 返回 net.Interface
	iface, err := net.InterfaceByName(ifName)
	if err != nil {
		return nil, fmt.Errorf("cannot get net.Interface for %q: %w", ifName, err)
	}
	return iface, nil
}

func dhClient4(ifname string, attempts int, verbose bool) (*netboot.BootConf, error) {
	if attempts < 1 {
		attempts = 1
	}

	client := client4.NewClient()

	var (
		conv []*dhcpv4.DHCPv4
		err  error
	)

	for attempt := 0; attempt < attempts; attempt++ {
		conv, err = client.Exchange(ifname)
		if err != nil && attempt < attempts {
			logrus.Warnf("runs a full DORA transaction err: %v", err)
			continue
		}
		break
	}
	if verbose {
		for _, m := range conv {
			logrus.Debug(m.Summary())
		}
	}
	if err != nil {
		return nil, err
	}
	// extract the network configuration
	return netboot.ConversationToNetconfv4(conv)
}
