package revm

import (
	"context"
	"errors"
	"golang.org/x/sync/errgroup"
	"net"
	"testing"
)

func TestServiceDisconnectDuringRequestedShutdown(t *testing.T) {
	for _, shuttingDown := range []bool{false, true} {
		vm := &VM{}
		vm.shutdownRequested.Store(shuttingDown)
		var group errgroup.Group
		forced := false
		vm.startHostService(&group, context.Background(), func(error) { forced = true }, func(context.Context) error { return net.ErrClosed })
		err := group.Wait()
		if shuttingDown {
			if err != nil || forced {
				t.Fatal("expected socket teardown replaced completed result", err)
			}
		} else if !errors.Is(err, net.ErrClosed) || !forced {
			t.Fatal("unexpected service failure was ignored")
		}
	}
}
