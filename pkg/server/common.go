//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"linuxvm/pkg/define"
	"linuxvm/pkg/ssh"
	"linuxvm/pkg/vmconfig"
	"net/http"
	"net/url"

	"github.com/sirupsen/logrus"
	gossh "golang.org/x/crypto/ssh"
)

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, code int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	if err := json.NewEncoder(w).Encode(value); err != nil {
		logrus.Errorf("failed to encode json response: %v", err)
	}
}

// ProcessOutput holds the output streams and error channel from a guest command.
type ProcessOutput struct {
	StdoutPipeReader *io.PipeReader
	StderrPipeReader *io.PipeReader
	errChan          chan error
}

// GuestExec executes a command in the guest VM via SSH.
// Returns a ProcessOutput that streams stdout/stderr and signals completion.
func GuestExec(ctx context.Context, vmc *vmconfig.VMConfig, bin string, args ...string) (*ProcessOutput, error) {
	endpoint, err := url.Parse(vmc.GVproxyEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to parse gvproxy endpoint: %w", err)
	}

	cfg := ssh.NewClientConfig(
		define.DefaultGuestAddr,
		uint16(define.DefaultGuestSSHDPort),
		define.DefaultGuestUser,
		vmc.SSHInfo.HostSSHKeyPairFile,
	).WithGVProxySocket(endpoint.Path)

	client, err := ssh.NewClient(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to guest: %w", err)
	}

	stdoutReader, stdoutWriter := io.Pipe()
	stderrReader, stderrWriter := io.Pipe()
	errChan := make(chan error, 1)

	go func() {
		defer func() {
			if err := client.Close(); err != nil {
				logrus.Errorf("failed to close ssh client: %v", err)
			}
		}()

		defer func() {
			close(errChan)
			_ = stdoutWriter.Close()
			_ = stderrWriter.Close()
		}()

		cmdSlice := append([]string{bin}, args...)
		errChan <- ssh.NewExecutor(client).Exec(ctx, &ssh.ExecOptions{
			Stdout:       stdoutWriter,
			Stderr:       stderrWriter,
			EnablePTY:    false,
			CancelSignal: gossh.SIGKILL,
		}, cmdSlice...)
	}()

	return &ProcessOutput{
		StdoutPipeReader: stdoutReader,
		StderrPipeReader: stderrReader,
		errChan:          errChan,
	}, nil
}
