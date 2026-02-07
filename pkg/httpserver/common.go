//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"linuxvm/pkg/define"
	"linuxvm/pkg/service"
	"linuxvm/pkg/vmconfig"
	"net/http"

	"al.essio.dev/pkg/shellescape"
	"github.com/sirupsen/logrus"
)

type ErrResponse struct {
	Error string `json:"error"`
}

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
	sshClient, err := service.MakeSSHClient(ctx, (*define.VMConfig)(vmc))
	if err != nil {
		return nil, err
	}

	stdoutReader, stdoutWriter := io.Pipe()
	stderrReader, stderrWriter := io.Pipe()
	errChan := make(chan error, 1)

	go func() {
		// 谁创建，谁关闭
		defer sshClient.Close()
		defer stdoutWriter.Close()
		defer stderrWriter.Close()
		errChan <- sshClient.RunWith(
			ctx,
			shellescape.QuoteCommand(append([]string{bin}, args...)),
			nil,
			stdoutWriter,
			stderrWriter)
	}()

	return &ProcessOutput{
		StdoutPipeReader: stdoutReader,
		StderrPipeReader: stderrReader,
		errChan:          errChan,
	}, nil
}
