package gvproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"net/http"
	"strings"

	"github.com/containers/gvisor-tap-vsock/pkg/types"
)

func ExposePort(ctx context.Context, controlAddr string, forward define.PortForward) error {
	req := types.ExposeRequest{
		Protocol: types.TransportProtocol(forward.Protocol),
		Local:    portForwardLocal(forward),
		Remote:   portForwardRemote(forward),
	}
	return postForwarder(ctx, controlAddr, "/services/forwarder/expose", req)
}

func UnexposePort(ctx context.Context, controlAddr string, forward define.PortForward) error {
	req := types.UnexposeRequest{
		Protocol: types.TransportProtocol(forward.Protocol),
		Local:    portForwardLocal(forward),
	}
	return postForwarder(ctx, controlAddr, "/services/forwarder/unexpose", req)
}

func postForwarder(ctx context.Context, controlAddr, path string, req any) error {
	addr, err := network.ParseUnixAddr(controlAddr)
	if err != nil {
		return fmt.Errorf("parse gvproxy control address: %w", err)
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal gvproxy request: %w", err)
	}

	client := network.NewUnixClient(addr.Path)
	defer client.Close()

	respBody, status, err := client.Post(path).JSON().Body(bytes.NewReader(body)).DoAndRead(ctx)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		msg := strings.TrimSpace(string(respBody))
		if msg == "" {
			msg = http.StatusText(status)
		}
		return fmt.Errorf("gvproxy forwarder returned %d: %s", status, msg)
	}
	return nil
}
