package server

import (
	"context"
	"fmt"
	"io"
	"linuxvm/pkg/network"
	"linuxvm/pkg/ssh"
	"linuxvm/pkg/vmconfig"

	"os/exec"

	"github.com/sirupsen/logrus"
	"github.com/tmaxmax/go-sse"
)

type ExecAction struct {
	Bin  string   `json:"bin,omitempty"`
	Args []string `json:"args,omitempty"`
	Env  []string `json:"env,omitempty"`
}

const (
	TypeOut = "out"
	TypeErr = "error"
)

func createSSEMsgAndPublish(msgType, msg string, sseServer *sse.Server, topic string) {
	m := &sse.Message{}
	m.AppendData(msg)
	m.Type = sse.Type(msgType)
	if err := sseServer.Publish(m, topic); err != nil {
		logrus.Warnf("Error publishing message: %v", err)
	}
}

type ExecInfo struct {
	outPip   io.Reader
	errPip   io.Reader
	process  *exec.Cmd
	exitChan chan error
}

const (
	topicKey = "topicKey"
)

var (
	sseServer *sse.Server
)

func GuestExec(ctx context.Context, vmc *vmconfig.VMConfig, bin string, args ...string) (*ExecInfo, error) {
	execInfo := &ExecInfo{}
	addr, err := network.ParseUnixAddr(vmc.GVproxyEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to parse gvproxy endpoint: %w", err)
	}

	cfg := ssh.NewCfg(vmc.SSHInfo.GuestAddr, vmc.SSHInfo.User, vmc.SSHInfo.Port, vmc.SSHInfo.HostSSHKeyPairFile)
	defer cfg.CleanUp.CleanIfErr(&err)
	cfg.SetPty(false)
	cfg.SetCmdLine(bin, args)
	err = cfg.Connect(ctx, addr.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gvproxy: %w", err)
	}
	if err = cfg.MakeStdPipe(); err != nil {
		return nil, fmt.Errorf("failed to make std pipe: %w", err)
	}
	execInfo.errPip = cfg.Stderr
	execInfo.outPip = cfg.Stdout

	go func() {
		execInfo.exitChan <- cfg.Run(ctx)
	}()

	return nil, nil
}

func LocalExec(ctx context.Context, bin string, args ...string) (*ExecInfo, error) {
	cmd := exec.CommandContext(ctx, bin, args...)

	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	errPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err = cmd.Start(); err != nil {
		return nil, err
	}
	execInfo := &ExecInfo{
		outPip:   outPipe,
		errPip:   errPipe,
		process:  cmd,
		exitChan: make(chan error, 1),
	}

	go func() {
		execInfo.exitChan <- execInfo.process.Wait()
	}()

	return execInfo, nil
}
