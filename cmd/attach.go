//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/ssh"
	"net/url"
	"path/filepath"

	"github.com/urfave/cli/v3"
)

var AttachConsole = cli.Command{
	Name:        "attach",
	Usage:       "attach to the console of the running rootfs",
	UsageText:   "attach [OPTIONS] [rootfs]",
	Description: "attach to the console of the running rootfs, provide the interactive shell of the rootfs",
	Action:      attachConsole,
}

func attachConsole(ctx context.Context, command *cli.Command) error {
	rootfs := command.Args().First()
	if rootfs == "" {
		return fmt.Errorf("no rootfs specified, please provide the rootfs path, e.g. %s /path/to/rootfs", command.Name)
	}

	vmc, err := define.LoadVMCFromFile(filepath.Join(rootfs, define.VMConfigFile))
	if err != nil {
		return err
	}

	//f, err := os.Create(vmc.SSHInfo.HostSSHKeyPairFile)
	//if err != nil {
	//	return err
	//}
	//defer f.Close()
	//_, err = f.WriteString(vmc.SSHInfo.HostSSHPrivateKey)
	//if err != nil {
	//	return err
	//}

	client, err := ssh.NewClient(define.DefaultGuestSSHAddr, define.DefaultGuestUser, define.DefaultGuestSSHPort, vmc.SSHInfo.HostSSHKeyPairFile, "true")
	if err != nil {
		return err
	}

	endpoint, err := url.Parse(vmc.GVproxyEndpoint)
	if err != nil {
		return fmt.Errorf("failed to parse gvproxy endpoint: %w", err)
	}

	return client.RunOverGVProxyVSock(ctx, endpoint.Path)
}
