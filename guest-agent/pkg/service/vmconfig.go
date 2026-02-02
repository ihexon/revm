package service

import (
	"context"
	"fmt"
	"guestAgent/pkg/vsock"

	"linuxvm/pkg/define"

	"github.com/sirupsen/logrus"
)

func getVMConfig(ctx context.Context) (*define.VMConfig, error) {
	svc := vsock.NewVSockService()
	defer func() {
		if err := svc.Close(); err != nil {
			logrus.Errorf("close vsock service error: %v", err)
		}
	}()

	vmc, err := svc.GetVMConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get vmconfig from vsock: %w", err)
	}

	return vmc, nil
}

func GetVMConfig(ctx context.Context) (*define.VMConfig, error) {
	vmc, err := getVMConfig(ctx)
	if err != nil {
		return nil, err
	}

	return vmc, vmc.WriteToJsonFile(define.VMConfigFilePathInGuest)
}
