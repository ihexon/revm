package service

import (
	"context"
	_ "embed"
	"fmt"
	"guestAgent/pkg/define"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/sirupsen/logrus"
)

//go:embed busybox.static
var elfData []byte

var (
	Busybox *Byx
	once    sync.Once
)

type Byx struct {
	path        string
	initialized bool
}

func InitializeBusybox(vmc *define.VMConfig) error {
	if err := os.MkdirAll(filepath.Dir(vmc.ExternalTools.LinuxTools.Busybox), 0755); err != nil {
		return err
	}

	once.Do(func() {
		Busybox = &Byx{
			path:        vmc.ExternalTools.LinuxTools.Busybox,
			initialized: true,
		}

	})

	return os.WriteFile(vmc.ExternalTools.LinuxTools.Busybox, elfData, 0755)
}

func (b *Byx) Exec(ctx context.Context, args ...string) error {
	if !b.initialized {
		return fmt.Errorf("busybox is not initialized")
	}

	cmd := exec.CommandContext(ctx, b.path, args...)

	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stderr
	}

	logrus.Debugf("busybox cmdline: %q", cmd.Args)

	return cmd.Run()
}
