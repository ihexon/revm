package revmcmd

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/urfave/cli/v3"
	"linuxvm/pkg/commanderr"
	"linuxvm/pkg/define"
	"slices"
)

//go:embed completions/revm.bash
var bashCompletion string

//go:embed completions/revm.zsh
var zshCompletion string

// Completion has its own entry point. Only this command enables the parser's
// completion mode; normal execution never interprets a guest argument as a
// completion request.
func newCompleteCommand() *cli.Command {
	return &cli.Command{
		Name: "_complete", Hidden: true, SkipFlagParsing: true,
		Action: func(ctx context.Context, c *cli.Command) error {
			args := c.Args().Slice()
			if slices.Contains(args, "--") {
				return nil
			}
			app := NewApp(c.Root().Name)
			app.EnableShellCompletion = true
			app.Writer, app.ErrWriter = c.Root().Writer, c.Root().ErrWriter
			// Keep command/flag definitions, but no executable actions.
			var disableActions func(*cli.Command)
			disableActions = func(command *cli.Command) {
				command.Action = nil
				for _, child := range command.Commands {
					disableActions(child)
				}
			}
			disableActions(app)
			args = append(append([]string{app.Name}, args...), "--generate-shell-completion")
			// The parser stores the parent command in ctx. Start a separate root
			// while retaining cancellation from the completion invocation.
			completionCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stop := context.AfterFunc(ctx, cancel)
			defer stop()
			return app.Run(completionCtx, args)
		},
	}
}

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
	c.Action = func(ctx context.Context, c *cli.Command) error {
		c.Writer = c.Root().Writer
		if c.Args().Len() != 1 {
			return commanderr.Usage(fmt.Errorf("completion requires one shell: bash, zsh or fish"))
		}
		var script string
		switch c.Args().First() {
		case "bash":
			script = bashCompletion
		case "zsh":
			script = zshCompletion
		case "fish":
			var err error
			script, err = c.Root().ToFishCompletion()
			if err != nil {
				return err
			}
			script += `
function __revm_dynamic_values
    set -l words (commandline -opc)
    command $words[1] _complete $words[2..-1]
end
complete -c revm -f -a '(__revm_dynamic_values)'
`
		default:
			return commanderr.Usage(fmt.Errorf("unsupported completion shell %q", c.Args().First()))
		}
		_, err := fmt.Fprint(c.Writer, script)
		return err
	}
}
