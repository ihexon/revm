//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package networkmode

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"

	"github.com/sirupsen/logrus"
)

// TSIMode implements the TSI (Transparent Socket Interception) network mode.
// This mode uses libkrun's built-in network capabilities without external network stack.
type TSIMode struct{}

func (t *TSIMode) StartNetworkStack(ctx context.Context, vmc *define.VMConfig) error {
	logrus.Info("TSI mode uses built-in networking, no external network stack needed")
	return nil
}

func (t *TSIMode) StartPodmanProxy(ctx context.Context, vmc *define.VMConfig) error {
	logrus.Info("TSI mode uses direct TCP for Podman API, no proxy needed")
	return nil
}

func (t *TSIMode) GetPodmanListenAddr(vmc *define.VMConfig) string {
	return fmt.Sprintf("%s:%d", define.LocalHost, define.GuestPodmanAPIPort)
}

func (t *TSIMode) String() string {
	return define.TSI.String()
}
