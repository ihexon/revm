//go:build darwin && (arm64 || amd64)

package vfkit

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/gvproxy"
	"linuxvm/pkg/vmconfig"
	"runtime"
	"strings"

	"github.com/Code-Hex/vz/v3"
	"github.com/crc-org/vfkit/pkg/config"
	"github.com/sirupsen/logrus"
)

type Stubber struct {
	vmc                 *vmconfig.VMConfig
	vfkitVirtualMachine *config.VirtualMachine
}

func NewStubber(vmc *vmconfig.VMConfig) *Stubber {
	return &Stubber{
		vmc: vmc,
	}
}

func (v *Stubber) StartNetwork(ctx context.Context) error {
	return gvproxy.StartNetworking(ctx, gvproxy.EndPoints{
		ControlEndpoints:    v.vmc.GVproxyEndpoint,
		VFKitSocketEndpoint: v.vmc.NetworkStackBackend,
	}, v.vmc.Stage.GVProxyChan)
}

func (v *Stubber) Create(ctx context.Context) error {
	virtualMachine, err := newVirtualMachine(v.vmc)
	if err != nil {
		return fmt.Errorf("failed to create virtual machine: %w", err)
	}
	v.vfkitVirtualMachine = virtualMachine
	vfkitCmdline, err := v.vfkitVirtualMachine.ToCmdLine()
	if err != nil {
		return fmt.Errorf("failed to convert virtual machine configure to cmdline: %w", err)
	}

	logrus.Debugf("vfkit cmdline: %q", vfkitCmdline)
	return nil
}

func (v *Stubber) Start(ctx context.Context) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	return nil
}

func (v *Stubber) Stop(ctx context.Context) error {
	// TODO implement me
	panic("implement me")
}

func (v *Stubber) GetVMConfigure() (*vmconfig.VMConfig, error) {
	if v.vmc == nil {
		return nil, fmt.Errorf("can not get vm config object, vmconfig is nil")
	}

	return v.vmc, nil
}

func newVirtualMachine(vmc *vmconfig.VMConfig) (*config.VirtualMachine, error) {
	bl := config.NewLinuxBootloader(
		vmc.Kernel,
		strings.Join(vmc.KernelCmdline, " "),
		vmc.Initrd,
	)

	vmConfig := config.NewVirtualMachine(
		uint(vmc.Cpus),
		vmc.MemoryInMB,
		bl,
	)

	vmConfig.Nested = false
	if vz.IsNestedVirtualizationSupported() {
		vmConfig.Nested = true
	}

	var devices []string

	// virtio-blk is the backend device for the data disk
	for _, disk := range vmc.BlkDevs {
		devices = append(devices, fmt.Sprintf("virtio-blk,path=%s", disk.Path))
	}

	// virtio-net is the backend device for the network
	devices = append(devices, fmt.Sprintf("virtio-net,unixSocketPath=%s", vmc.NetworkStackBackend))

	// virtio-rng is the backend device for the random number generator
	devices = append(devices, "virtio-rng")

	// add virtio-balloon
	devices = append(devices, "virtio-balloon")

	// add virtio-vsock
	devices = append(devices, fmt.Sprintf("virtio-vsock,port=%d,socketURL=%s,listen", define.DefaultVSockPort, vmc.IgnProvisionerAddr))

	// virtio-fs is the backend device for the host shared directory
	for _, mount := range vmc.Mounts {
		devices = append(devices, fmt.Sprintf("virtio-fs,sharedDir=%s,mountTag=%s", mount.Source, mount.Tag))
	}

	// add virtio-serial console
	devices = append(devices, "virtio-serial,stdio")

	if err := vmConfig.AddDevicesFromCmdLine(devices); err != nil {
		return nil, fmt.Errorf("failed to add devices: %w", err)
	}

	return vmConfig, nil
}
