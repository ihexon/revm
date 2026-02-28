//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vmbuilder

import (
	"context"
	"fmt"
	"linuxvm/pkg/event"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/static_resources"
	"os"
	"path/filepath"
)

func (v *VM) withBuiltInAlpineRootfs(ctx context.Context, pathMgr *PathManager) error {
	if v.WorkspacePath == "" {
		return fmt.Errorf("workspace path is empty")
	}

	alpineRootfsDir := pathMgr.GetRootfsDir()
	if err := os.MkdirAll(alpineRootfsDir, 0755); err != nil {
		return err
	}

	if err := static_resources.ExtractBuiltinRootfs(ctx, alpineRootfsDir); err != nil {
		return err
	}
	event.Emit(event.RootfsExtractedReady)

	return v.withUserProvidedRootfs(ctx, alpineRootfsDir)
}

func (v *VM) withUserProvidedRootfs(ctx context.Context, rootfsPath string) error {
	if rootfsPath == "" {
		return fmt.Errorf("rootfs path is empty")
	}

	rootfsPath, err := filepath.Abs(filepath.Clean(rootfsPath))
	if err != nil {
		return err
	}

	_, err = os.Lstat(filepath.Join(rootfsPath, "bin", "sh"))
	if err != nil {
		return fmt.Errorf("rootfs path %q does not contain shell interpreter /bin/sh: %w", rootfsPath, err)
	}

	v.RootFS = rootfsPath

	return nil
}

func (v *VM) withMountUserHome(ctx context.Context) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return v.withUserProvidedMounts([]string{fmt.Sprintf("%s:%s", homeDir, homeDir)})
}

func (v *VM) withUserProvidedMounts(dirs []string) error {
	if len(dirs) == 0 || dirs == nil {
		return nil
	}

	mounts := filesystem.CmdLineMountToMounts(dirs)
	for i := range mounts {
		p, err := filepath.Abs(filepath.Clean(mounts[i].Source))
		if err != nil {
			return fmt.Errorf("failed to resolve mount source %q: %w", mounts[i].Source, err)
		}
		mounts[i].Source = p
	}
	v.Mounts = append(v.Mounts, mounts...)
	return nil
}
