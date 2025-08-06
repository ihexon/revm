//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package filesystem

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type Mount struct {
	ReadOnly bool   `json:"readOnly"`
	Source   string `json:"source"`
	Tag      string `json:"tag"`
	Target   string `json:"target"`
	Type     string `json:"type"`
}

func CmdLineMountToMounts(mnts []string) []Mount {
	var mounts []Mount //nolint:prealloc
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
	return paths[0]
}

func extractMountOptions(paths []string) bool {
	readonly := false
	if len(paths) > 2 { //nolint:mnd
		options := paths[2]
		volopts := strings.Split(options, ",")
		for _, o := range volopts {
			switch o {
			case "rw":
				readonly = false
			case "ro":
				readonly = true
			}
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

func (v VirtIoFs) ToMount() Mount {
	return Mount{
		ReadOnly: v.ReadOnly,
		Tag:      v.Tag,
		Source:   v.Source,
		Target:   v.Target,
		Type:     v.Kind(),
	}
}

const virtIOFs = "virtiofs"

func (v VirtIoFs) Kind() string {
	return virtIOFs
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
	return strings.SplitN(volume, ":", 3) //nolint:mnd
}

func extractTargetPath(paths []string) string {
	if len(paths) > 1 {
		return paths[1]
	}
	return paths[0]
}
