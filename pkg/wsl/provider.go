package wsl

import (
	"context"
	"linuxvm/pkg/vmconfig"
)

type Provider struct{}

func (p Provider) CreateVM(ctx context.Context, opts vmconfig.VMConfig) {
	//TODO implement me
	panic("implement me")
}

func (p Provider) StartVM(ctx context.Context, vmc vmconfig.VMConfig) {
	//TODO implement me
	panic("implement me")
}

func (p Provider) StopVM(ctx context.Context, vmc vmconfig.VMConfig) {
	//TODO implement me
	panic("implement me")
}

func (p Provider) MountHostDir2VM(ctx context.Context, vmc vmconfig.VMConfig) {
	//TODO implement me
	panic("implement me")
}

func (p Provider) StartNetworkProvider(ctx context.Context, vmc vmconfig.VMConfig) {
	//TODO implement me
	panic("implement me")
}
