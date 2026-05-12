package revmcmd

import (
	"context"
	"errors"
	"os"
	"os/exec"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

func Run(app *cli.Command) {
	if err := app.Run(context.Background(), os.Args); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		logrus.Error(err)
		os.Exit(1)
	}
}
