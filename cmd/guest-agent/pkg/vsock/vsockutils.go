//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vsock

import (
	"context"
	"encoding/json"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
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

func (v *Service) GetVMConfig(ctx context.Context) (*define.Machine, error) {
	body, status, err := v.client.Get(define.RestAPIVMConfigURL).DoAndRead(ctx)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("GET vmconfig returned %d", status)
	}

	vmc := &define.Machine{}
	if err = json.Unmarshal(body, vmc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal vmconfig: %w", err)
	}

	return vmc, nil
}

func (v *Service) PostReady(ctx context.Context, serviceName string) error {
	resp, err := v.client.Post("/ready/" + serviceName).Do(ctx)
	if err != nil {
		return err
	}
	defer network.CloseResponse(resp)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("POST /ready/%s returned %d", serviceName, resp.StatusCode)
	}
	return nil
}
