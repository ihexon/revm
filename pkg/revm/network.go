//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

func (v *machineBuilder) configureNetwork() error {
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
