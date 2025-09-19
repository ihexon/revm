package services

import (
	"context"
	"fmt"
	"linuxvm/cmd/bootstrap/pkg/path"
)

func DownloadLinuxUtils(ctx context.Context) error {
	fileList := []string{
		"busybox.static",
		"dropbear",
		"dropbearkey",
	}

	v := NewVSockService()

	for _, fileName := range fileList {
		err := v.DownloadFile(ctx, fileName, path.GetGuestLinuxUtilsBinPath(fileName), false)
		if err != nil {
			return fmt.Errorf("failed to download linux utils binary: %w", err)
		}
	}

	return nil
}
