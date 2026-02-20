//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"linuxvm/pkg/ssh_v2"
	"linuxvm/pkg/vmbuilder"
	"net/http"
	"os"
	"os/signal"
	"syscall"
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
func GuestExec(ctx context.Context, vmc *vmbuilder.VM, bin string, args ...string) (*ProcessOutput, error) {
	sshClient, err := MakeSSHClient(ctx, (*define.Machine)(vmc))
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

func ListenSignal(ctx context.Context) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, os.Interrupt)
	defer signal.Stop(sigCh)
	select {
	case <-ctx.Done():
		return nil
	case sig := <-sigCh:
		return fmt.Errorf("received signal: %s", sig)
	}
}

func WatchParentProcess(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if os.Getppid() == 1 {
				return fmt.Errorf("parent process exited, shutting down")
			}
		}
	}
}

func WatchMachineExitChannel(ctx context.Context, vmc *define.Machine) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-vmc.StopCh:
		return fmt.Errorf("VM stop requested via API")
	}
}

func StartIgnitionService(ctx context.Context, vmc *define.Machine) error {
	ignSrv := NewIgnServer(vmc)
	return ignSrv.Start(ctx)
}

func StartNetworkStack(ctx context.Context, vmc *define.Machine) error {
	switch vmc.VirtualNetworkMode {
	case define.GVISOR:
		mode := &GVisorMode{}
		return mode.StartNetworkStack(ctx, vmc)
	case define.TSI:
		mode := &TSIMode{}
		return mode.StartNetworkStack(ctx, vmc)
	}
	return nil
}

func StartPodmanAPIProxy(ctx context.Context, vmc *define.Machine) error {
	if vmc.RunMode != define.ContainerMode.String() {
		return nil
	}

	go func() {
		select {
		case <-ctx.Done():
			return
		case <-vmc.Readiness.PodmanReady:
			logrus.Infof("Podman API proxy listen in: %q", vmc.PodmanInfo.PodmanProxyAddr)
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-vmc.Readiness.VNetHostReady:
		switch vmc.VirtualNetworkMode {
		case define.GVISOR:
			mode := &GVisorMode{}
			return mode.StartPodmanProxy(ctx, vmc)
		case define.TSI:
			mode := &TSIMode{}
			return mode.StartPodmanProxy(ctx, vmc)
		}
	}

	return nil
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
		guestAddr = fmt.Sprintf("%s:%d", define.GuestIP, define.GuestSSHServerPort)
		dialOpts = append(dialOpts, ssh_v2.WithTunnel(gvCtlAddr.Path))
	} else {
		// TSI mode: direct TCP connection
		guestAddr = fmt.Sprintf("%s:%d", define.LocalHost, define.GuestSSHServerPort)
	}

	return ssh_v2.Dial(ctx, guestAddr, dialOpts...)
}
