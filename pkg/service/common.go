//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package service

import (
	"context"
	"encoding/json"
	"io"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"linuxvm/pkg/ssh_v2"
	"net"
	"net/http"
	"time"

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
func GuestExec(ctx context.Context, vmc *define.Machine, bin string, args ...string) (*ProcessOutput, error) {
	sshClient, err := MakeSSHClient(ctx, vmc)
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

func MakeSSHClient(ctx context.Context, vmc *define.Machine) (*ssh_v2.Client, error) {
	// Build SSH dial options based on network mode
	dialOpts := []ssh_v2.Option{
		ssh_v2.WithUser(define.DefaultGuestUser),
		ssh_v2.WithPrivateKey(vmc.SSHInfo.HostSSHPrivateKeyFile),
		ssh_v2.WithTimeout(2 * time.Second),
		ssh_v2.WithKeepalive(2 * time.Second),
	}

	var guestAddr string
	if vmc.VirtualNetworkMode == define.GVISOR {
		// GVISOR mode: use tunnel through gvproxy
		gvCtlAddr, err := network.ParseUnixAddr(vmc.GVPCtlAddr)
		if err != nil {
			return nil, err
		}
		_, portStr, _ := net.SplitHostPort(vmc.SSHInfo.GuestSSHServerListenAddr)
		guestAddr = net.JoinHostPort(define.GuestIP, portStr)
		dialOpts = append(dialOpts, ssh_v2.WithTunnel(gvCtlAddr.Path))
	} else {
		// TSI mode: direct TCP connection
		guestAddr = vmc.SSHInfo.GuestSSHServerListenAddr
	}

	return ssh_v2.Dial(ctx, guestAddr, dialOpts...)
}
