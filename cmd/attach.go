//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/ssh"
	"net/url"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

var AttachConsole = cli.Command{
	Name:        "attach",
	Description: "attach to the console of the running VM",
	Flags:       []cli.Flag{},
	Before:      earlyStage,
	Action:      attachConsole,
}

func attachConsole(ctx context.Context, command *cli.Command) error {
	rootfs := command.Args().First()
	vmc, err := define.LoadVMCFromFile(filepath.Join(rootfs, define.VMConfigFile))
	if err != nil {
		return err
	}

	client, err := ssh.NewClient(define.DefaultGuestSSHAddr, define.DefaultGuestUser, define.DefaultGuestSSHPort, vmc.HostSSHKeyFile, "true")
	if err != nil {
		return err
	}

	endpoint, err := url.Parse(vmc.GVproxyEndpoint)
	if err != nil {
		return fmt.Errorf("failed to parse gvproxy endpoint: %w", err)
	}
	logrus.Infof("gvproxy endpoint: %q", endpoint.Path)

	return client.RunOverGVProxyVSock(ctx, endpoint.Path)

}
