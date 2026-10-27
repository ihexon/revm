//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"fmt"
	"linuxvm/pkg/define"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

func (v *machineBuilder) configureNetwork() error {
	if v.Network == define.NetworkTSI {
		data, err := os.ReadFile("/etc/resolv.conf")
		if err != nil {
			return fmt.Errorf("read host DNS configuration for TSI: %w", err)
		}
		v.ResolvConf, err = tsiResolvConf(string(data))
		return err
	}
	logrus.Infof("Configuring gvisor-tap-vsock network mode")
	pathMgr := v.pathMgr
	controlPath := pathMgr.GetGVPCtlSocketFile()
	v.GVPCtlAddr = (&url.URL{Scheme: "unix", Path: controlPath}).String()
	v.GVPVNetAddr = fmt.Sprintf("unixgram://%s", pathMgr.GetVNetSocketFile())
	v.GVPNotifyAddr = fmt.Sprintf("unix://%s", pathMgr.GetGVPNotifySocketFile())

	for _, path := range []string{controlPath, pathMgr.GetVNetSocketFile(), pathMgr.GetGVPNotifySocketFile()} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale network socket %q: %w", path, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(controlPath), 0755); err != nil {
		return err
	}

	return nil
}

// The pinned libkrun misencodes IPv6 scope IDs. Keep usable host DNS servers
// rather than passing scoped addresses that can abort guest resolver startup.
func tsiResolvConf(config string) (string, error) {
	var lines []string
	servers := 0
	for _, line := range strings.Split(config, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "nameserver" {
			if strings.Contains(fields[1], "%") {
				continue
			}
			servers++
		}
		lines = append(lines, line)
	}
	if servers == 0 {
		return "", fmt.Errorf("TSI requires a nameserver without an IPv6 scope in host /etc/resolv.conf")
	}
	return strings.Join(lines, "\n"), nil
}
