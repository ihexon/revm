//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vm

import (
	"context"
	"errors"
	"linuxvm/pkg/define"
	"linuxvm/pkg/libkrun"
	"linuxvm/pkg/vmconfig"
)

type Provider interface {
	StartNetwork(ctx context.Context) error
	Create(ctx context.Context) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	GetVMConfigure() (*vmconfig.VMConfig, error)
}

func Get(vmc *vmconfig.VMConfig) Provider {
	switch vmc.RunMode {
	case define.KernelMode.String():
		// TODO: implement the vfkit provider
		panic(errors.New("vfkit provider is not implemented yet"))
	default:
		return libkrun.NewAppleHyperVisor(vmc)
	}
}
