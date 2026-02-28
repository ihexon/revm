//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package filesystem

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"linuxvm/pkg/define"
	"strings"
)

func CmdLineMountToMounts(mnts []string) []define.Mount {
	var mounts []define.Mount //nolint:prealloc
	for i, volume := range mnts {
		if volume == "" {
			continue
		}
		_, source, target, readOnly := SplitVolume(i, volume)
		m := NewVirtIoFsMount(source, target, readOnly).ToMount()
		mounts = append(mounts, m)
	}
	return mounts
}

func SplitVolume(idx int, volume string) (string, string, string, bool) {
	tag := fmt.Sprintf("mnt-%d", idx)
	paths := pathsFromVolume(volume)
	source := extractSourcePath(paths)
	target := extractTargetPath(paths)
	readonly := extractMountOptions(paths)
	return tag, source, target, readonly
}

func extractSourcePath(paths []string) string {
	if len(paths) > 1 {
		return paths[0]
	}
	// Single path: strip options after comma
	src, _, _ := strings.Cut(paths[0], ",")
	return src
}

func extractMountOptions(paths []string) bool {
	// Options are after the comma in the last part
	last := paths[len(paths)-1]
	_, opts, found := strings.Cut(last, ",")
	if !found {
		return false
	}
	readonly := false
	volopts := strings.Split(opts, ",")
	for _, o := range volopts {
		switch o {
		case "rw":
			readonly = false
		case "ro":
			readonly = true
		}
	}
	return readonly
}

type VirtIoFs struct {
	ReadOnly bool
	Tag      string
	Source   string
	Target   string
}

func (v VirtIoFs) ToMount() define.Mount {
	return define.Mount{
		ReadOnly: v.ReadOnly,
		Tag:      v.Tag,
		Source:   v.Source,
		Target:   v.Target,
		Type:     v.Kind(),
	}
}

const VirtIOFs = "virtiofs"

func (v VirtIoFs) Kind() string {
	return VirtIOFs
}

func NewVirtIoFsMount(src, target string, readOnly bool) VirtIoFs {
	vfs := VirtIoFs{
		ReadOnly: readOnly,
		Source:   src,
		Target:   target,
	}
	vfs.Tag = vfs.generateTag()
	return vfs
}

// generateTag generates a tag for VirtIOFs mounts.
func (v VirtIoFs) generateTag() string {
	sum := sha256.Sum256([]byte(v.Target))
	stringSum := hex.EncodeToString(sum[:])
	return stringSum[:36]
}

func pathsFromVolume(volume string) []string {
	return strings.SplitN(volume, ":", 2) //nolint:mnd
}

func extractTargetPath(paths []string) string {
	if len(paths) > 1 {
		target, _, _ := strings.Cut(paths[1], ",")
		return target
	}
	// Single path: target = source, strip options after comma
	target, _, _ := strings.Cut(paths[0], ",")
	return target
}
