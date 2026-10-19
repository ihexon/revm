//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package guestcontrol

import (
	"context"
	"fmt"
	"io"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"time"
)

type Target struct {
	CID        uint32
	Port       uint32
	UnixSocket string
}

func Shutdown(ctx context.Context, target Target) error {
	client := newClient(target)
	defer client.Close()
	resp, err := client.Post("/v1/shutdown").Do(ctx)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("guest shutdown returned %d: %s", resp.StatusCode, body)
	}
	return nil
}

func DefaultTarget() Target {
	return Target{CID: define.GuestVSockCID, Port: define.GuestControlPort}
}

func newClient(target Target) *network.Client {
	if target.UnixSocket != "" {
		return network.NewUnixClient(target.UnixSocket, network.WithTimeout(2*time.Second))
	}
	if target.CID == 0 {
		target.CID = define.GuestVSockCID
	}
	if target.Port == 0 {
		target.Port = define.GuestControlPort
	}
	return network.NewVSockClient(target.CID, target.Port, network.WithTimeout(2*time.Second))
}
