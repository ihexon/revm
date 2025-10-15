//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vm

import (
	"context"
	"fmt"
	"linuxvm/pkg/libkrun"
	"linuxvm/pkg/vfkit"
	"linuxvm/pkg/vmconfig"
	"runtime"
)

type Provider interface {
	StartNetwork(ctx context.Context) error
	Create(ctx context.Context) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	GetVMConfigure() (*vmconfig.VMConfig, error)
}

func Get(vmc *vmconfig.VMConfig) (Provider, error) {
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		return libkrun.NewStubber(vmc), nil
	}

	if runtime.GOOS == "darwin" && runtime.GOARCH == "amd64" {
		return vfkit.NewStubber(vmc), nil
	}

	return nil, fmt.Errorf("not support this platform")
}
