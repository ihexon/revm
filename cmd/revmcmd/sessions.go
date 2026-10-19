package revmcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"linuxvm/pkg/commanderr"
	"linuxvm/pkg/revm"
	"time"

	"github.com/urfave/cli/v3"
)

func timeoutContext(ctx context.Context, c *cli.Command) (context.Context, context.CancelFunc, error) {
	d := c.Duration("timeout")
	if d < 0 {
		return nil, nil, commanderr.Usage(fmt.Errorf("--timeout must not be negative"))
	}
	if d == 0 {
		ctx, cancel := context.WithCancel(ctx)
		return ctx, cancel, nil
	}
	ctx, cancel := context.WithTimeout(ctx, d)
	return ctx, cancel, nil
}
func sessionCommands() []*cli.Command {
	return []*cli.Command{
		{Name: "ls", Usage: "list sessions and their latest state", Flags: []cli.Flag{&cli.BoolFlag{Name: "json"}}, Action: func(ctx context.Context, c *cli.Command) error {
			if err := noArgs(c); err != nil {
				return err
			}
			views, err := revm.ListSessions()
			if err != nil {
				return err
			}
			if c.Bool("json") {
				return json.NewEncoder(c.Writer).Encode(views)
			}
			fmt.Fprintln(c.Writer, "ID\tSTATUS\tINSTANCE")
			for _, v := range views {
				fmt.Fprintf(c.Writer, "%s\t%s\t%s\n", v.ID, v.Status, v.InstanceID)
			}
			return nil
		}},
		{Name: "inspect", Usage: "print session configuration and last result as JSON", Action: func(ctx context.Context, c *cli.Command) error {
			id, err := sessionArg(c)
			if err != nil {
				return err
			}
			view, err := revm.InspectSession(id)
			if err != nil {
				return err
			}
			e := json.NewEncoder(c.Writer)
			e.SetIndent("", "  ")
			return e.Encode(view)
		}},
		{Name: "stop", Usage: "request shutdown and wait for this instance to release its resources", Flags: []cli.Flag{&cli.DurationFlag{Name: "timeout", Value: 15 * time.Second, Usage: "maximum wait; VM grace budget is set when starting it"}}, Action: func(ctx context.Context, c *cli.Command) error {
			id, err := sessionArg(c)
			if err != nil {
				return err
			}
			ctx, cancel, err := timeoutContext(ctx, c)
			if err != nil {
				return err
			}
			defer cancel()
			return revm.StopSession(ctx, id)
		}},
		{Name: "wait", Usage: "wait for this instance's exit and return its status", Flags: []cli.Flag{&cli.BoolFlag{Name: "ready", Usage: "wait for guest execution readiness instead of exit"}, &cli.DurationFlag{Name: "timeout", Usage: "maximum wait; zero waits indefinitely"}}, Action: func(ctx context.Context, c *cli.Command) error {
			id, err := sessionArg(c)
			if err != nil {
				return err
			}
			ctx, cancel, err := timeoutContext(ctx, c)
			if err != nil {
				return err
			}
			defer cancel()
			return revm.WaitSession(ctx, id, c.Bool("ready"))
		}},
		{Name: "rm", Usage: "remove one stopped session and its logs; preserve persistent data", Action: func(ctx context.Context, c *cli.Command) error {
			id, err := sessionArg(c)
			if err != nil {
				return err
			}
			return revm.RemoveSession(id)
		}},
		{Name: "logs", Usage: "read the latest instance log", Flags: []cli.Flag{&cli.BoolFlag{Name: "guest", Usage: "read vm.log instead of revm.log"}, &cli.BoolFlag{Name: "follow", Aliases: []string{"f"}, Usage: "follow until this instance stops"}, &cli.StringFlag{Name: "instance", Usage: "read a specific instance ID"}}, Action: func(ctx context.Context, c *cli.Command) error {
			id, err := sessionArg(c)
			if err != nil {
				return err
			}
			return revm.SessionLogs(ctx, id, c.String("instance"), c.Bool("guest"), c.Bool("follow"), c.Writer)
		}},
	}
}

func sessionArg(c *cli.Command) (string, error) {
	id, err := oneArg(c)
	if err != nil {
		return "", err
	}
	if err = revm.ValidateSessionID(id); err != nil {
		return "", commanderr.Usage(err)
	}
	return id, nil
}
