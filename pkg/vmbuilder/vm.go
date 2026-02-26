//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vmbuilder

import (
	"fmt"
	"io"
	"linuxvm/pkg/define"
	"linuxvm/pkg/filesystem"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofrs/flock"
	"github.com/sirupsen/logrus"
)

type VM struct {
	define.Machine
	fileLock *flock.Flock
}

func NewVirtualMachine(mode define.RunMode) *VM {
	return &VM{
		Machine: define.Machine{
			RunMode:       mode.String(),
			XATTRSRawDisk: map[string]string{},
			StopCh:        make(chan struct{}),
			Readiness:     define.NewReadiness(),
		},
	}
}

func (v *VM) setupWorkspace(workspacePath string) error {
	if workspacePath == "" {
		return fmt.Errorf("workspace path is empty")
	}

	workspacePath, err := filepath.Abs(filepath.Clean(workspacePath))
	if err != nil {
		return err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	underTmp := strings.HasPrefix(workspacePath, "/tmp")
	underHome := strings.HasPrefix(workspacePath, homeDir)
	if !underTmp && !underHome {
		return fmt.Errorf("workspace must be under /tmp or home directory (%s), got %q", homeDir, workspacePath)
	}

	v.WorkspacePath = workspacePath

	if err = os.MkdirAll(v.WorkspacePath, 0755); err != nil {
		return err
	}

	return v.lock()
}

func (v *VM) lock() error {
	fileLock := flock.New(filepath.Join(v.WorkspacePath, ".lock"))

	ifLocked, err := fileLock.TryLock()
	if err != nil {
		return fmt.Errorf("get lock failed: %w", err)
	}

	if !ifLocked {
		return fmt.Errorf("workspace %q is locked by another instance", fileLock.Path())
	}

	v.fileLock = fileLock
	return nil
}

func (v *VM) setupLogLevel(level string) error {
	l, err := logrus.ParseLevel(level)
	if err != nil {
		return fmt.Errorf("invalid log level: %w", err)
	}

	logrus.SetLevel(l)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		ForceColors:     true,
		TimestampFormat: "2006-01-02 15:04:05.000",
	})

	v.LogFilePath = filepath.Join(v.WorkspacePath, "logs", "vm.log")

	if err := os.MkdirAll(filepath.Dir(v.LogFilePath), 0755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	if info, err := os.Stat(v.LogFilePath); err == nil && info.Size() > int64(filesystem.MiB(10).ToBytes()) {
		_ = os.Truncate(v.LogFilePath, 0)
	}

	f, err := os.OpenFile(v.LogFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	logrus.SetOutput(io.MultiWriter(os.Stderr, f))

	return nil
}

func (v *VM) withResources(memoryInMB uint64, cpus int8) error {
	if cpus <= 0 {
		return fmt.Errorf("1 cpu cores is the minimum value")
	}

	if memoryInMB < 512 {
		return fmt.Errorf("512MB of memory is the minimum value")
	}

	v.MemoryInMB = memoryInMB
	v.Cpus = cpus

	return nil
}

func (v *VM) setupCmdLine(workdir, bin string, args, envs []string) error {
	if v.RunMode != define.RootFsMode.String() {
		return fmt.Errorf("expect run mode %q, but got %q", define.RootFsMode.String(), v.RunMode)
	}

	if v.RootFS == "" {
		return fmt.Errorf("rootfs path is empty")
	}

	if workdir == "" {
		return fmt.Errorf("workdir path is empty")
	}

	if bin == "" {
		return fmt.Errorf("bin path is empty")
	}

	for _, arg := range args {
		if strings.Contains(arg, ";") || strings.Contains(arg, "|") ||
			strings.Contains(arg, "&") || strings.Contains(arg, "`") {
			return fmt.Errorf("dangerous shell metacharacters in argument: %s", arg)
		}
	}

	if v.ProxySetting.Use {
		envs = append(envs, "http_proxy="+v.ProxySetting.HTTPProxy)
		envs = append(envs, "https_proxy="+v.ProxySetting.HTTPSProxy)
	}

	v.Cmdline = define.Cmdline{
		Bin:     bin,
		Args:    args,
		Envs:    envs,
		WorkDir: workdir,
	}

	return nil
}
