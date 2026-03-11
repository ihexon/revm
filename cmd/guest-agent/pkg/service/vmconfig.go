package service

import (
	"context"
	"encoding/json"
	"fmt"
	"guestAgent/pkg/vsock"
	"linuxvm/pkg/define"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

func getVMConfig(ctx context.Context) (*define.Machine, error) {
	ctx, cancel := context.WithTimeout(ctx, define.DefaultProbeTimeout)
	defer cancel()

	ticker := time.NewTicker(define.DefaultTimeTicker)
	defer ticker.Stop()

	var lastErr error
	for {
		svc := vsock.NewVSockService()
		vmc, err := svc.GetVMConfig(ctx)
		_ = svc.Close()
		if err == nil {
			return vmc, nil
		}

		lastErr = err
		logrus.Debugf("get vmconfig attempt failed: %v, retrying...", err)

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("failed to get vmconfig from vsock: %w (last: %v)", ctx.Err(), lastErr)
		case <-ticker.C:
		}
	}
}

func GetVMConfig(ctx context.Context) (*define.Machine, error) {
	vmc, err := getVMConfig(ctx)
	if err != nil {
		return nil, err
	}

	return vmc, WriteToJsonFile(vmc, define.VMConfigFilePathInGuest)
}

func WriteToJsonFile(vmc *define.Machine, file string) error {
	b, err := json.Marshal(vmc)
	if err != nil {
		return fmt.Errorf("failed to marshal vmconfig: %w", err)
	}

	return os.WriteFile(file, b, 0644)
}
