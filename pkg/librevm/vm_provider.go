//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/krunrunner"
	"runtime"
)

// newProvider creates a RunnerProvider for the current platform, delegating
// all libkrun CGo calls to a child process to prevent libkrun's exit() from
// terminating the main process before cleanup can run.
func newProvider(mc *define.Machine) (*krunrunner.RunnerProvider, error) {
	switch {
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
	case runtime.GOOS == "linux" && (runtime.GOARCH == "arm64" || runtime.GOARCH == "amd64"):
	default:
		return nil, fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return krunrunner.NewRunnerProvider(mc), nil
}
