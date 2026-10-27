package service

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/protocol"

	"golang.org/x/sync/errgroup"
)

// RunGuest owns the required services. The host ends its own process to stop
// the VM; there is no guest shutdown RPC or reboot choreography.
func RunGuest(ctx context.Context, vmc *protocol.GuestSpec) error {
	g, services := errgroup.WithContext(ctx)
	container := vmc.RunMode == define.ContainerMode.String()
	if !container && vmc.RunMode != define.RootFsMode.String() {
		return fmt.Errorf("unsupported mode %q", vmc.RunMode)
	}
	g.Go(func() error { return StartGuestControlServer(services, container) })
	g.Go(func() error { return SyncRTCTime(services) })
	if container {
		g.Go(func() error { return StartGuestPodmanService(services, vmc) })
	}
	return g.Wait()
}
