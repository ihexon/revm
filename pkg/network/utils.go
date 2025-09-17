//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package network

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"linuxvm/pkg/define"
	"linuxvm/pkg/system"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/mdlayher/vsock"
	"github.com/sirupsen/logrus"
)

func GetVMConfigFromVSockHTTP(ctx context.Context) (*define.VMConfig, error) {
	client := createVSockHTTPClient()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://vsock/", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to GET vsock: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	vmc := &define.VMConfig{}
	if err = json.Unmarshal(data, vmc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal vmconfig: %w", err)
	}

	if err = vmc.WriteToJsonFile(filepath.Join("/", define.VMConfigFile)); err != nil {
		return nil, fmt.Errorf("failed to write vmconfig to file: %w", err)
	}

	return vmc, nil
}

func createVSockHTTPClient() *http.Client {
	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				result := make(chan struct {
					c   net.Conn
					err error
				}, 1)

				go func() {
					c, err := vsock.Dial(2, define.DefaultVSockPort, nil)
					result <- struct {
						c   net.Conn
						err error
					}{c, err}
				}()

				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case r := <-result:
					return r.c, r.err
				}
			},
		},
	}
	return client
}

const vsockFileEndpoint = "http://vsock/3rd/linux/bin/"

func Download3rdFileFromVSockHttp(ctx context.Context, target, saveTo string, overWrite bool) error {
	if !overWrite && system.IsPathExist(saveTo) {
		logrus.Debugf("%q already exist, skip", target)
		if err := os.Chmod(saveTo, 0755); err != nil {
			return fmt.Errorf("failed to chmod 0755 %q: %w", saveTo, err)
		}

		return nil
	}

	client := createVSockHTTPClient()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, vsockFileEndpoint+target, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to GET vsock: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	tmpFile := saveTo + "." + uuid.NewString()
	if err = os.MkdirAll(filepath.Dir(tmpFile), 0755); err != nil {
		return fmt.Errorf("failed to create dir: %w", err)
	}

	if err = os.WriteFile(tmpFile, data, 0755); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tmpFile, saveTo); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}
