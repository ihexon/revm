package revmcmd

import (
	"context"
	"fmt"
	"linuxvm/pkg/revm"
	"strings"

	"github.com/urfave/cli/v3"
)

// Keep usage and examples beside the command tree; generated completion and
// --help use the same definitions as execution.
func configureHelp(app *cli.Command) {
	app.Description = "Run one Linux command in a temporary VM. Successful temporary runs clean up automatically; failed runs keep diagnostics.\n\nExamples:\n  revm run -- uname -a\n  printf 'hello\\n' | revm run -- cat\n  revm run --id dev --mount \"$PWD:/work\" --workdir /work -- sh"
	examples := map[string]string{
		"run":           "Arguments after -- are passed unchanged. Use sh -c for shell syntax.\n\nExamples:\n  revm run -- uname -a\n  revm run --mount \"$PWD:/work\" --workdir /work -- sh\n  revm run -- sh -c 'echo hello; exit 42'",
		"podman":        "Runs in the foreground until stopped. Create the volume first; it survives session removal.\n\nExamples:\n  revm volume create data\n  revm podman --id containers --volume data --podman-api /tmp/revm-podman.sock\n  # In another terminal:\n  CONTAINER_HOST=unix:///tmp/revm-podman.sock podman info",
		"exec":          "Without COMMAND, opens /bin/sh. The session must already be running.\n\nExamples:\n  revm exec --id dev\n  revm exec --id dev --workdir /tmp -- sh -c 'pwd'",
		"ctl":           "Manage TCP port forwarding for a running gvisor session.\n\nExamples:\n  revm ctl --id dev --list-port --json\n  revm ctl --id dev --port-export 127.0.0.1:8080:8000\n  revm ctl --id dev --port-unexport 127.0.0.1:8080",
		"ls":            "Example:\n  revm ls --json",
		"inspect":       "Example:\n  revm inspect dev",
		"stop":          "The timeout limits this client's wait. Shutdown grace is set with --stop-timeout when starting the VM.\n\nExample:\n  revm stop --timeout 15s dev",
		"wait":          "Waits for the selected instance, even if the session name is later reused.\n\nExamples:\n  revm wait --ready --timeout 30s dev\n  revm wait dev  # returns its recorded exit status",
		"logs":          "Reads revm lifecycle logs; --guest selects the VM boot/service log. Command stdout/stderr are streamed directly and are not recorded here.\n\nExamples:\n  revm logs --tail 50 dev\n  revm logs --guest --tail 20 --follow dev",
		"rm":            "Requires a stopped session. Deletes its records and logs; preserves volumes and external disks.\n\nExample:\n  revm rm dev",
		"volume":        "Persistent disks are independent of sessions. Select one explicitly with revm podman --volume NAME.\n\nExample:\n  revm volume create data",
		"volume create": "Example:\n  revm volume create data",
		"volume ls":     "Example:\n  revm volume ls --json",
		"volume rm":     "Permanently deletes this volume's data. Refuses a volume in use.\n\nExample:\n  revm volume rm data",
		"cache":         "Example:\n  revm cache prune",
		"cache prune":   "Reclaims inactive runtime directories and rootfs copies. Preserves logs, results, volumes and external disks.\n\nExample:\n  revm cache prune",
	}
	var visit func(*cli.Command, string)
	visit = func(c *cli.Command, path string) {
		c.ShellComplete = completeValues
		if description, ok := examples[path]; ok {
			c.Description = description
		}
		switch path {
		case "inspect", "stop", "wait", "logs", "rm":
			c.ArgsUsage = "SESSION"
		case "volume create", "volume rm":
			c.ArgsUsage = "NAME"
		}
		for _, child := range c.Commands {
			visit(child, strings.TrimSpace(path+" "+child.Name))
		}
	}
	visit(app, "")
}

func completeValues(ctx context.Context, c *cli.Command) {
	// The root's positional arguments retain the child command's raw flags,
	// including an incomplete flag value that the child parser cannot accept.
	args := c.Root().Args().Slice()
	last := ""
	if len(args) > 0 {
		last = args[len(args)-1]
	}
	kind, prefix := "", ""
	switch {
	case last == "--id":
		kind = "session"
	case last == "--volume":
		kind = "volume"
	case strings.HasPrefix(last, "--id="):
		kind, prefix = "session", "--id="
	case strings.HasPrefix(last, "--volume="):
		kind, prefix = "volume", "--volume="
	case strings.HasPrefix(last, "-"):
		for _, f := range c.VisibleFlags() {
			for _, name := range f.Names() {
				flag := "--" + name
				if len(name) == 1 {
					flag = "-" + name
				}
				if strings.HasPrefix(flag, last) {
					fmt.Fprintln(c.Root().Writer, flag)
				}
			}
		}
	}
	if kind == "" && c.Args().Len() == 0 {
		switch c.Name {
		case "inspect", "stop", "wait", "logs", "rm":
			kind = "session"
			if len(args) > 0 && args[0] == "volume" {
				kind = "volume"
			}
		}
	}
	switch kind {
	case "session":
		views, _ := revm.ListSessions()
		for _, v := range views {
			fmt.Fprintln(c.Root().Writer, prefix+v.ID)
		}
	case "volume":
		volumes, _ := revm.ListVolumes()
		for _, v := range volumes {
			fmt.Fprintln(c.Root().Writer, prefix+v.Name)
		}
	default:
		if !strings.HasPrefix(last, "-") {
			cli.DefaultCompleteWithFlags(ctx, c)
		}
	}
}
