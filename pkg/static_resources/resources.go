package static_resources

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"linuxvm/pkg/archiver"
	"linuxvm/pkg/define"
	"os"
	"path/filepath"

	libarchivego "github.com/ihexon/libarchive-go"
)

//go:embed raw_disks/ext4.raw.tar
var BuiltinRawDiskBytes []byte

//go:embed guest-agent/guest-agent
var GuestAgentBytes []byte

//go:embed rootfs/rootfs.tar.zst
var RootfsBytes []byte

//go:embed e2fsprogs/**
var e2fsTools embed.FS

func GetExecutableDir() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("failed to eval symlinks: %w", err)
	}

	selfDir := filepath.Dir(path)

	return selfDir, nil
}

func GetLibexecPath() (string, error) {
	binDir, err := GetExecutableDir()
	if err != nil {
		return "", err
	}
	parentDir := filepath.Dir(binDir)
	return filepath.Join(parentDir, define.LibexecDirName), nil
}

// Get3rdBinPath get 3rd bin path from $BIN/../libexec/
func Get3rdBinPath(name string) (string, error) {
	libexecPath, err := GetLibexecPath()
	if err != nil {
		return "", err
	}

	binPath := filepath.Join(libexecPath, name)
	f, err := os.Stat(binPath)
	if err != nil {
		return "", fmt.Errorf("failed to stat %q: %w", binPath, err)
	}

	mode := f.Mode()
	if !mode.IsRegular() {
		return "", fmt.Errorf("%q is not a regular file (mode=%s)", binPath, mode.String())
	}

	if mode&0o111 == 0 {
		return "", fmt.Errorf("%q is not executable (mode=%s)", binPath, mode.String())
	}

	return binPath, nil
}

func GetBuiltinTool(workspace, name string) (string, error) {
	targetFilePath := filepath.Join(workspace, "e2fsprogs", name)
	if fd, err := os.Stat(targetFilePath); err == nil {
		if fd.Mode().IsRegular() && fd.Mode().Perm()&0111 != 0 {
			return targetFilePath, nil
		}
	}

	data, err := e2fsTools.ReadFile("e2fsprogs/" + name)
	if err != nil {
		return "", err
	}

	err = os.MkdirAll(filepath.Dir(targetFilePath), 0755)
	if err != nil {
		return "", err
	}

	return targetFilePath, os.WriteFile(targetFilePath, data, 0755)
}

func ExtractEmbeddedRawDisk(ctx context.Context, filePath string) error {
	filePath, err := filepath.Abs(filepath.Clean(filePath))
	if err != nil {
		return err
	}

	if err = os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}

	return archiver.NewTarInstance().
		Stdin(bytes.NewReader(BuiltinRawDiskBytes)).
		Members("ext4.raw").
		RemoveOld(true).
		Transform(fmt.Sprintf("|ext4.raw|%s|", filepath.Base(filePath))).
		Unarchive(ctx, "-", filepath.Dir(filePath))
}

func ExtractBuiltinRootfs(ctx context.Context, dstDir string) error {
	dstDir, err := filepath.Abs(filepath.Clean(dstDir))
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}

	defer func() {
		RootfsBytes = nil
	}()

	return libarchivego.NewArchiver().SetReader(bytes.NewReader(RootfsBytes)).SetChdir(dstDir).SetSparse(true).ModeX(ctx)
}
