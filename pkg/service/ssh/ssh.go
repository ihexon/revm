//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package ssh

import (
	"context"
	"io"
	"linuxvm/pkg/network"
	ssh "linuxvm/pkg/ssh"
	"net"
	"time"

	"al.essio.dev/pkg/shellescape"
)

type ProcessOutput struct {
	StdoutPipeReader *io.PipeReader
	StderrPipeReader *io.PipeReader
	ErrChan          chan error
}

type Target struct {
	User                     string
	PrivateKeyFile           string
	UseGVProxyTunnel         bool
	GVPCtlAddr               string
	GuestSSHServerListenAddr string
	GuestTunnelHost          string
}

func GuestExec(ctx context.Context, target Target, bin string, args ...string) (*ProcessOutput, error) {
	sshClient, err := MakeSSHClient(ctx, target)
	if err != nil {
		return nil, err
	}
	stdoutReader, stdoutWriter := io.Pipe()
	stderrReader, stderrWriter := io.Pipe()
	errChan := make(chan error, 1)
	go func() {
		defer sshClient.Close()
		defer stdoutWriter.Close()
		defer stderrWriter.Close()
		errChan <- sshClient.RunWith(ctx, shellescape.QuoteCommand(append([]string{bin}, args...)), nil, stdoutWriter, stderrWriter)
	}()
	return &ProcessOutput{StdoutPipeReader: stdoutReader, StderrPipeReader: stderrReader, ErrChan: errChan}, nil
}

func MakeSSHClient(ctx context.Context, target Target) (*ssh.Client, error) {
	user := target.User
	if user == "" {
		user = "root"
	}
	dialOpts := []ssh.Option{ssh.WithUser(user), ssh.WithPrivateKey(target.PrivateKeyFile), ssh.WithTimeout(2 * time.Second), ssh.WithKeepalive(2 * time.Second)}
	var guestAddr string
	if target.UseGVProxyTunnel {
		gvCtlAddr, err := network.ParseUnixAddr(target.GVPCtlAddr)
		if err != nil {
			return nil, err
		}
		_, portStr, _ := net.SplitHostPort(target.GuestSSHServerListenAddr)
		guestHost := target.GuestTunnelHost
		if guestHost == "" {
			guestHost = "192.168.127.2"
		}
		guestAddr = net.JoinHostPort(guestHost, portStr)
		dialOpts = append(dialOpts, ssh.WithTunnel(gvCtlAddr.Path))
	} else {
		guestAddr = target.GuestSSHServerListenAddr
	}
	return ssh.Dial(ctx, guestAddr, dialOpts...)
}
