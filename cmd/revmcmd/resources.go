package revmcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/urfave/cli/v3"
	"linuxvm/pkg/commanderr"
	"linuxvm/pkg/revm"
)

func oneArg(c *cli.Command) (string, error) {
	if c.Args().Len() != 1 {
		return "", commanderr.Usage(fmt.Errorf("%s requires exactly one name", c.Name))
	}
	return c.Args().First(), nil
}
func noArgs(c *cli.Command) error {
	if c.Args().Len() != 0 {
		return commanderr.Usage(fmt.Errorf("%s does not accept positional arguments", c.Name))
	}
	return nil
}
func resourceCommands() []*cli.Command {
	return []*cli.Command{
		{Name: "volume", Usage: "manage persistent disks independently of sessions", Commands: []*cli.Command{
			{Name: "ls", Usage: "list managed volumes", Flags: []cli.Flag{&cli.BoolFlag{Name: "json"}}, Action: func(ctx context.Context, c *cli.Command) error {
				if err := noArgs(c); err != nil {
					return err
				}
				list, err := revm.ListVolumes()
				if err != nil {
					return err
				}
				if c.Bool("json") {
					return json.NewEncoder(c.Writer).Encode(list)
				}
				for _, v := range list {
					fmt.Fprintf(c.Writer, "%s\t%s\t%s\n", v.Name, v.ID, v.DefaultSession)
				}
				return nil
			}},
			{Name: "rm", Usage: "delete one unused managed volume; retained session references become missing", Action: func(ctx context.Context, c *cli.Command) error {
				name, err := oneArg(c)
				if err != nil {
					return err
				}
				return revm.RemoveVolume(name)
			}},
		}},
		{Name: "cache", Usage: "manage disposable runtime and cache resources", Commands: []*cli.Command{
			{Name: "prune", Usage: "remove inactive instance resources; preserve persistent and legacy data", Action: func(ctx context.Context, c *cli.Command) error {
				if err := noArgs(c); err != nil {
					return err
				}
				paths, err := revm.PruneCache()
				for _, p := range paths {
					fmt.Fprintln(c.Writer, p)
				}
				return err
			}},
		}},
		{Name: "migrate", Usage: "copy and verify a stopped legacy session into the current layout; keep the source", Action: func(ctx context.Context, c *cli.Command) error {
			name, err := oneArg(c)
			if err != nil {
				return err
			}
			s, err := revm.MigrateSession(ctx, name)
			if err != nil {
				return err
			}
			return json.NewEncoder(c.Writer).Encode(s)
		}},
	}
}
