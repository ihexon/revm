package service

import (
	"context"
	"encoding/json"
	"fmt"
	"guestAgent/pkg/vsock"
	"linuxvm/pkg/define"
	"os"

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

	logrus.Infof("VM config received (mode: %s)", vmc.RunMode)
	return vmc, WriteToJsonFile(vmc, define.VMConfigFilePathInGuest)
}

func WriteToJsonFile(vmc *define.VMConfig, file string) error {
	b, err := json.Marshal(vmc)
	if err != nil {
		return fmt.Errorf("failed to marshal vmconfig: %v", err)
	}

	return os.WriteFile(file, b, 0644)
}
