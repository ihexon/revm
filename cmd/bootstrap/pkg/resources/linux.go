package resources

import (
	"context"
	"embed"
	"fmt"
	"linuxvm/cmd/bootstrap/pkg/path"
	"linuxvm/pkg/define"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

//go:embed ".bin/**"
var linuxToolsEmbed embed.FS

func DownloadLinuxTools(ctx context.Context) error {
	fileList := []string{
		"busybox.static",
		"dropbear",
		"dropbearkey",
	}

	if err := os.MkdirAll(define.GuestLinuxUtilsBinDir, 0755); err != nil {
		return err
	}

	for _, fileName := range fileList {
		data, err := linuxToolsEmbed.ReadFile(filepath.Join(".bin", fileName))
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
		logrus.Debugf("downloading file %q", fileName)
		if err = os.WriteFile(path.GetToolsPath3rd(fileName), data, 0755); err != nil {
			return err
		}
	}

	return nil
}
