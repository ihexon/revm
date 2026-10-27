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
			logrus.Warnf("read host DNS configuration for TSI: %v; continuing without host DNS", err)
			data = nil
		}
		v.ResolvConf = tsiResolvConf(string(data))
		return nil
	}
	logrus.Infof("Configuring gvisor-tap-vsock network mode")
	pathMgr := v.pathMgr
	controlPath := pathMgr.GetGVPCtlSocketFile()
	v.GVPCtlAddr = (&url.URL{Scheme: "unix", Path: controlPath}).String()
	v.GVPVNetAddr = fmt.Sprintf("unixgram://%s", pathMgr.GetVNetSocketFile())

	for _, path := range []string{controlPath, pathMgr.GetVNetSocketFile()} {
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
func tsiResolvConf(config string) string {
	var lines []string
	for _, line := range strings.Split(config, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "nameserver" && strings.Contains(fields[1], "%") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
