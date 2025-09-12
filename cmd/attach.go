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
	Usage:       "attach to the guest and running command",
	UsageText:   "attach [OPTIONS] [rootfs] [cmdline]",
	Description: "attach to the console of the running rootfs, provide the interactive shell of the rootfs",
	Action:      attachConsole,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    define.FlagPTY,
			Aliases: []string{"tty"},
			Usage:   "enable pseudo-terminal",
			Value:   false,
		},
	},
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

	endpoint, err := url.Parse(vmc.GVproxyEndpoint)
	if err != nil {
		return fmt.Errorf("failed to parse gvproxy endpoint: %w", err)
	}

	if command.Args().Len() < 2 {
		return fmt.Errorf("no cmdline specified, please provide the cmdline, e.g. %s /path/to/rootfs /bin/bash", command.Name)
	}

	cmdline := command.Args().Tail()

	cfg := ssh.NewCfg(vmc.SSHInfo.GuestAddr, vmc.SSHInfo.User, vmc.SSHInfo.Port, vmc.SSHInfo.HostSSHKeyPairFile)
	defer cfg.CleanUp.CleanIfErr(&err)

	cfg.SetCmdLine(cmdline[0], cmdline[1:])

	if command.Bool(define.FlagPTY) {
		cfg.SetPty(true)
	}

	if err = cfg.Connect(ctx, endpoint.Path); err != nil {
		return fmt.Errorf("failed to connect to %s: %w", endpoint.Path, err)
	}

	if err := cfg.MakeStdPipe(); err != nil {
		return fmt.Errorf("failed to make std pipe: %w", err)
	}

	if cfg.IsPty() {
		resetFunc, err := cfg.RequestPTY(ctx)
		if err != nil {
			return err
		}
		defer resetFunc()
	}

	logrus.Infof("run ssh session's cmdline")
	if err = cfg.Run(ctx); err != nil {
		return fmt.Errorf("failed to run: %w", err)
	}

	return nil
}
