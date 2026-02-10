//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vsock

import (
	"context"
	"encoding/json"
	"fmt"
	"linuxvm/pkg/define"
	"net/http"
	"time"
)

// Service provides high-level VSock HTTP operations
type Service struct {
	client *HTTPClient
}

// Close closes the VSock service client
func (v *Service) Close() error {
	return v.client.Close()
}

func NewVSockService() *Service {
	return &Service{
		client: NewVSockHTTPClientV2(2, define.DefaultVSockPort, 2*time.Second),
	}
}

func (v *Service) GetVMConfig(ctx context.Context) (*define.VMConfig, error) {
	resp, err := v.client.GetJSON(ctx, define.RestAPIVMConfigURL)
	if err != nil {
		return nil, err
	}

	vmc := &define.VMConfig{}
	if err = json.Unmarshal(resp, vmc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal vmconfig: %w", err)
	}

	return vmc, nil
}

func (v *Service) PostReady(ctx context.Context, serviceName string) error {
	resp, err := v.client.Post(ctx, "/ready/"+serviceName, "", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("POST /ready/%s returned %d", serviceName, resp.StatusCode)
	}
	return nil
}
