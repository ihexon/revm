package gvproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"net/http"
	"strings"
)

func ListPorts(ctx context.Context, socket string) ([]define.PortMapping, error) {
	body, status, err := network.UnixRequest(ctx, socket, http.MethodGet, "/services/forwarder/all", nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("gvproxy returned %d: %s", status, strings.TrimSpace(string(body)))
	}
	ports := []define.PortMapping{}
	if err := json.Unmarshal(body, &ports); err != nil {
		return nil, fmt.Errorf("decode port mappings: %w", err)
	}
	return ports, nil
}
