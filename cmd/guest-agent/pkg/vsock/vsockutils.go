//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vsock

import (
	"context"
	"encoding/json"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"linuxvm/pkg/protocol"
	"net/http"
	"time"
)

// Service provides high-level VSock HTTP operations
type Service struct {
	client *network.Client
}

// Close closes the VSock service client
func (v *Service) Close() error {
	return v.client.Close()
}

func NewVSockService() *Service {
	return &Service{
		client: network.NewVSockClient(2, define.DefaultVSockPort, network.WithTimeout(2*time.Second)),
	}
}

func (v *Service) GetVMConfig(ctx context.Context) (*protocol.GuestSpec, error) {
	body, status, err := v.client.Get(define.RestAPIVMConfigURL).DoAndRead(ctx)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("GET vmconfig returned %d", status)
	}

	vmc := &protocol.GuestSpec{}
	if err = json.Unmarshal(body, vmc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal vmconfig: %w", err)
	}
	if vmc.SchemaVersion != protocol.GuestSpecVersion {
		return nil, fmt.Errorf("unsupported guest spec schema version: %d", vmc.SchemaVersion)
	}

	return vmc, nil
}
