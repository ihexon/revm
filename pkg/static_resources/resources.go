package static_resources

import (
	"bytes"
	"context"
	_ "embed"
	"os"
	"path/filepath"

	libarchive_go "linuxvm/pkg/libarchive"
)

//go:embed raw_disks/ext4.raw.tar
var BuiltinRawDiskBytes []byte

//go:embed rootfs/rootfs.tar.zst
var RootfsBytes []byte

func ExtractEmbeddedRawDisk(ctx context.Context, targetPath string) error {
	targetPath, err := filepath.Abs(filepath.Clean(targetPath))
	if err != nil {
		return err
	}
	baseDir, fileName := filepath.Split(targetPath)

	if err = os.MkdirAll(baseDir, 0755); err != nil {
		return err
	}

	return libarchive_go.NewArchiver().
		SetReader(bytes.NewReader(BuiltinRawDiskBytes)).
		SetFastRead(true).
		SetSparse(true).
		WithPattern("ext4.raw").
		SetChdir(baseDir).
		WithTransform("ext4.raw", fileName).
		ModeX(ctx)
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

	return libarchive_go.NewArchiver().SetReader(bytes.NewReader(RootfsBytes)).SetChdir(dstDir).SetSparse(true).ModeX(ctx)
}
