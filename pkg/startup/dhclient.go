//go:build linux

package startup

import (
	"fmt"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/client4"
	"github.com/insomniacslk/dhcp/netboot"
	"log"
	"net"
	"time"
)

func DHClient4(ifName string, attempts int, verbose bool) error {
	_, err := BringInterfaceUp(ifName)
	if err != nil {
		return fmt.Errorf("failed to bring interface %q up: %w", ifName, err)
	}

	bootConf, err := dhClient4(ifName, attempts, verbose)
	if err != nil {
		return fmt.Errorf("failed to get dhcp config: %w", err)
	}

	return netboot.ConfigureInterface(ifName, &bootConf.NetConf)
}

func BringInterfaceUp(ifName string) (_ *net.Interface, err error) {
	return netboot.IfUp(ifName, 2*time.Second)
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
		log.Printf("Attempt %d of %d", attempt+1, attempts)
		conv, err = client.Exchange(ifname)
		if err != nil && attempt < attempts {
			log.Printf("Error: %v", err)
			continue
		}
		break
	}
	if verbose {
		for _, m := range conv {
			log.Print(m.Summary())
		}
	}
	if err != nil {
		return nil, err
	}
	// extract the network configuration
	return netboot.ConversationToNetconfv4(conv)
}
