package revmcmd

import (
	"context"
	"fmt"
	"github.com/urfave/cli/v3"
	"linuxvm/pkg/commanderr"
	"linuxvm/pkg/define"
)

func versionString() string {
	version, commit, date := define.Version, define.CommitID, define.BuildDate
	if version == "" {
		version = "dev"
	}
	if commit == "" {
		commit = "unknown"
	}
	if date == "" {
		date = "unknown"
	}
	return fmt.Sprintf("%s (commit %s, built %s)", version, commit, date)
}
func configureCompletion(c *cli.Command) {
	c.Hidden = false
	configureErrors(c)
	action := c.Action
	c.Action = func(ctx context.Context, c *cli.Command) error {
		c.Writer = c.Root().Writer
		if c.Args().Len() != 1 {
			return commanderr.Usage(fmt.Errorf("completion requires one shell: bash, zsh or fish"))
		}
		switch c.Args().First() {
		case "bash", "zsh", "fish":
		default:
			return commanderr.Usage(fmt.Errorf("unsupported completion shell %q", c.Args().First()))
		}
		return action(ctx, c)
	}
}
