//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package ssh

import (
	"context"
	"io"
	"linuxvm/pkg/define"
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

func GuestExec(ctx context.Context, vmc *define.Machine, bin string, args ...string) (*ProcessOutput, error) {
	sshClient, err := MakeSSHClient(ctx, vmc)
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

func MakeSSHClient(ctx context.Context, vmc *define.Machine) (*ssh.Client, error) {
	dialOpts := []ssh.Option{ssh.WithUser(define.DefaultGuestUser), ssh.WithPrivateKey(vmc.SSHInfo.HostSSHPrivateKeyFile), ssh.WithTimeout(2 * time.Second), ssh.WithKeepalive(2 * time.Second)}
	var guestAddr string
	if vmc.VirtualNetworkMode == define.GVISOR {
		gvCtlAddr, err := network.ParseUnixAddr(vmc.GVPCtlAddr)
		if err != nil {
			return nil, err
		}
		_, portStr, _ := net.SplitHostPort(vmc.SSHInfo.GuestSSHServerListenAddr)
		guestAddr = net.JoinHostPort(define.GuestIP, portStr)
		dialOpts = append(dialOpts, ssh.WithTunnel(gvCtlAddr.Path))
	} else {
		guestAddr = vmc.SSHInfo.GuestSSHServerListenAddr
	}
	return ssh.Dial(ctx, guestAddr, dialOpts...)
}
