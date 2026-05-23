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

func ListPorts(ctx context.Context, controlAddr string) ([]define.PortMapping, error) {
	var ports []types.ExposeRequest
	if err := getForwarder(ctx, controlAddr, "/services/forwarder/all", &ports); err != nil {
		return nil, err
	}

	mappings := make([]define.PortMapping, 0, len(ports))
	for _, port := range ports {
		mappings = append(mappings, define.PortMapping{
			Protocol: string(port.Protocol),
			Local:    port.Local,
			Remote:   port.Remote,
		})
	}
	return mappings, nil
}

func getForwarder(ctx context.Context, controlAddr, path string, out any) error {
	addr, err := network.ParseUnixAddr(controlAddr)
	if err != nil {
		return fmt.Errorf("parse gvproxy control address: %w", err)
	}

	client := network.NewUnixClient(addr.Path)
	defer client.Close()

	respBody, status, err := client.Get(path).JSON().DoAndRead(ctx)
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
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode gvproxy response: %w", err)
	}
	return nil
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
