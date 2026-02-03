package service

import (
	"context"
	"guestAgent/pkg/network"
)

const (
	eth0     = "eth0"
	attempts = 3
)

func ConfigureNetwork(ctx context.Context) error {
	return network.DHClient4(ctx, eth0, attempts)
}
