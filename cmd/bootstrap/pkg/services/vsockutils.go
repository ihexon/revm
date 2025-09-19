//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"linuxvm/cmd/bootstrap/pkg/path"
	"linuxvm/cmd/bootstrap/pkg/vsock"
	"linuxvm/pkg/define"
	"net/http"
	url2 "net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

type ServiceName string

const (
	ServicePodman ServiceName = "podman"
	ServiceSSHD   ServiceName = "sshd"
)

func (s ServiceName) String() string {
	return string(s)
}

// VSockService provides high-level VSock HTTP operations
type VSockService struct {
	vmconfigURL    string
	FileServerURL  string
	PodmanReadyURL string
	SSHReadyURL    string
	client         *vsock.HTTPClient
}

// InformServiceReady notifies the host that a guest service is ready
func (v *VSockService) InformServiceReady(ctx context.Context, service ServiceName) error {
	path := fmt.Sprintf("/ready/%s", service.String())
	resp, err := v.client.Get(ctx, path)
	if err != nil {
		return fmt.Errorf("failed to inform service ready: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("service ready notification failed with status: %d", resp.StatusCode)
	}

	return nil
}

// Close closes the VSock service client
func (v *VSockService) Close() error {
	return v.client.Close()
}

func NewVSockService() *VSockService {
	return &VSockService{
		vmconfigURL:    define.RestAPIVMConfigURL,
		FileServerURL:  define.RestAPI3rdFileServerURL,
		PodmanReadyURL: define.RestAPIPodmanReadyURL,
		SSHReadyURL:    define.RestAPISSHReadyURL,
		client:         vsock.NewVSockHTTPClientV2(2, define.DefaultVSockPort, 2*time.Second),
	}
}

func (v *VSockService) GetVMConfig(ctx context.Context) (*define.VMConfig, error) {
	resp, err := v.client.GetJSON(ctx, v.vmconfigURL)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = v.Close()
	}()

	vmc := &define.VMConfig{}
	if err = json.Unmarshal(resp, vmc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal vmconfig: %w", err)
	}

	if err = vmc.WriteToJsonFile(filepath.Join("/", define.VMConfigFile)); err != nil {
		return nil, fmt.Errorf("failed to write vmconfig to file: %w", err)
	}

	return vmc, nil
}

func (v *VSockService) DownloadFile(ctx context.Context, filename, savePath string, overwrite bool) error {
	if !overwrite && path.IsPathExist(savePath) {
		logrus.Debugf("file %q already exists, skipping download", filename)
		return nil
	}

	url, err := url2.JoinPath(v.FileServerURL, filename)
	if err != nil {
		return fmt.Errorf("failed to join path %q: %w", filename, err)
	}

	logrus.Infof("downloading file %q save to %q", filename, savePath)

	resp, err := v.client.Get(ctx, url)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	return saveResponseToFile(resp, savePath)
}

func saveResponseToFile(resp *http.Response, savePath string) error {
	tempFile := savePath + ".tmp." + uuid.New().String()

	file, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		_ = file.Close()
		_ = os.Remove(tempFile)
	}()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	if err := os.Chmod(tempFile, 0755); err != nil {
		return fmt.Errorf("failed to chmod file: %w", err)
	}

	if err := os.Rename(tempFile, savePath); err != nil {
		return fmt.Errorf("failed to rename file: %w", err)
	}

	return nil
}
