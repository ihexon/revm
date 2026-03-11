//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package krunrunner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"linuxvm/pkg/define"

	"github.com/sirupsen/logrus"
)

// RunnerProvider implements interfaces.VMMProvider by delegating
// all libkrun CGo calls to a child process (krun-runner).
type RunnerProvider struct {
	mc  *define.Machine
	mu  sync.Mutex
	cmd *exec.Cmd
}

func NewRunnerProvider(mc *define.Machine) *RunnerProvider {
	return &RunnerProvider{mc: mc}
}

func (p *RunnerProvider) Start(ctx context.Context) error {
	// 序列化 Machine config
	configJSON, err := json.Marshal(p.mc)
	if err != nil {
		return fmt.Errorf("marshal machine config: %w", err)
	}

	// 创建 pipe 传递 config
	pr, pw, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("create config pipe: %w", err)
	}

	// 查找 krun-runner 二进制路径
	runnerBin, err := resolveRunnerPath()
	if err != nil {
		pr.Close()
		pw.Close()
		return err
	}

	// 构造命令：Linux 通过 ld-linux 加载，macOS 直接运行
	cmd := buildCommand(runnerBin)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = upsertEnv(os.Environ(), define.EnvLogLevel, logrus.GetLevel().String())
	cmd.ExtraFiles = []*os.File{pr} // fd 3

	if err := cmd.Start(); err != nil {
		pr.Close()
		pw.Close()
		return fmt.Errorf("start krun-runner: %w", err)
	}

	// 子进程已启动，立即记录 cmd，使 Stop() 可以 kill 它
	p.mu.Lock()
	p.cmd = cmd
	p.mu.Unlock()

	// 关闭 reader 端 (子进程已继承)，写入 config，关闭 writer
	pr.Close()
	if _, err := pw.Write(configJSON); err != nil {
		pw.Close()
		cmd.Process.Kill() //nolint:errcheck
		return fmt.Errorf("write config to krun-runner: %w", err)
	}
	pw.Close()

	return cmd.Wait()
}

func (p *RunnerProvider) Stop() error {
	p.mu.Lock()
	cmd := p.cmd
	p.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		return cmd.Process.Kill()
	}
	return nil
}

// resolveRunnerPath 查找 krun-runner 二进制
func resolveRunnerPath() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	// 布局: bin/revm (或 bin/.revm)，helper/krun-runner
	runnerBin := filepath.Join(filepath.Dir(execPath), "..", "helper", "krun-runner")
	if _, err := os.Stat(runnerBin); err != nil {
		return "", fmt.Errorf("krun-runner not found at %s: %w", runnerBin, err)
	}
	return runnerBin, nil
}

// buildCommand 根据平台构造执行命令
// Linux: 通过 bundled ld-linux 加载以确保使用正确的共享库
// macOS: 直接执行（dylib 通过 @loader_path 引用）
func buildCommand(runnerBin string) *exec.Cmd {
	if runtime.GOOS == "linux" {
		helperDir := filepath.Dir(runnerBin)
		libDir := filepath.Join(helperDir, "..", "lib")
		ldLinux := filepath.Join(libDir, ldLinuxName())
		return exec.Command(ldLinux, "--library-path", libDir, runnerBin)
	}
	return exec.Command(runnerBin)
}

func ldLinuxName() string {
	if runtime.GOARCH == "amd64" {
		return "ld-linux-x86-64.so.2"
	}
	return "ld-linux-aarch64.so.1"
}

func upsertEnv(env []string, key, value string) []string {
	prefix := key + "="
	filtered := make([]string, 0, len(env)+1)
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			continue
		}
		filtered = append(filtered, item)
	}
	return append(filtered, prefix+value)
}
