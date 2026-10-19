package revmcmd

import (
	"context"
	"encoding/json"
	"errors"
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
			{Name: "create", Usage: "create a persistent Podman volume", Action: func(ctx context.Context, c *cli.Command) error {
				name, err := oneArg(c)
				if err != nil {
					return err
				}
				if err := revm.ValidateSessionID(name); err != nil {
					return commanderr.Usage(err)
				}
				v, err := revm.CreateVolume(ctx, name)
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(c.Writer, v.Name)
				return err
			}},
			{Name: "ls", Usage: "list volume paths, usage state, capacity and allocated space", Flags: []cli.Flag{&cli.BoolFlag{Name: "json", Usage: "print structured volume records with sizes in bytes"}}, Action: func(ctx context.Context, c *cli.Command) error {
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
				return writeVolumes(c.Writer, list)
			}},
			{Name: "rm", Usage: "delete one unused managed volume", Action: func(ctx context.Context, c *cli.Command) error {
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
					if _, writeErr := fmt.Fprintln(c.Writer, p); writeErr != nil {
						return errors.Join(err, writeErr)
					}
				}
				return err
			}},
		}},
	}
}
