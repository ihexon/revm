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
	c.Usage = "print a shell completion script for bash, zsh or fish"
	c.ArgsUsage = "SHELL"
	c.Description = "Examples:\n  source <(revm completion bash)  # bash-completion required\n  source <(revm completion zsh)   # run compinit first\n  revm completion fish > ~/.config/fish/completions/revm.fish"
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
		if err := action(ctx, c); err != nil {
			return err
		}
		if c.Args().First() == "fish" {
			_, err := fmt.Fprint(c.Writer, `
function __revm_dynamic_values
    set -l words (commandline -opc)
    if contains -- -- $words; return; end
    command $words --generate-shell-completion
end
complete -c revm -f -a '(__revm_dynamic_values)'
`)
			return err
		}
		return nil
	}
}
