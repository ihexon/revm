//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/disk"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/static_resources"
	"os"
	gopath "path"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

type RawDiskSpec struct {
	Path    string `toml:"path,omitempty"     json:"path,omitempty"`
	UUID    string `toml:"uuid,omitempty"     json:"uuid,omitempty"`
	Version string `toml:"version,omitempty"  json:"version,omitempty"`
	MountTo string `toml:"mount_to,omitempty" json:"mountTo,omitempty"`
}

var (
	newRawDiskManager      = func() (disk.Manager, error) { return disk.NewBlkManager() }
	newRawDiskXattrManager = filesystem.NewXattrManager
	extractEmbeddedRAWDisk = static_resources.ExtractEmbeddedRawDisk
)

func ParseRawDiskSpecs(specs []string) ([]RawDiskSpec, error) {
	parsed := make([]RawDiskSpec, 0, len(specs))
	for _, spec := range specs {
		rawDisk, err := ParseRawDiskSpec(spec)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, rawDisk)
	}
	return parsed, nil
}

func ParseRawDiskSpec(input string) (RawDiskSpec, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return RawDiskSpec{}, fmt.Errorf("raw disk spec cannot be empty")
	}

	parts := strings.Split(input, ",")
	spec := RawDiskSpec{
		Path: strings.TrimSpace(parts[0]),
	}
	if spec.Path == "" {
		return RawDiskSpec{}, fmt.Errorf("raw disk path cannot be empty")
	}

	seen := map[string]struct{}{}
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if part == "" {
			return RawDiskSpec{}, fmt.Errorf("raw disk spec %q contains an empty option", input)
		}

		key, value, ok := strings.Cut(part, "=")
		if !ok {
			return RawDiskSpec{}, fmt.Errorf("raw disk option %q must use key=value syntax", part)
		}

		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			return RawDiskSpec{}, fmt.Errorf("raw disk option %q must have non-empty key and value", part)
		}
		if _, exists := seen[key]; exists {
			return RawDiskSpec{}, fmt.Errorf("raw disk option %q is duplicated", key)
		}
		seen[key] = struct{}{}

		switch key {
		case "uuid":
			if _, err := uuid.Parse(value); err != nil {
				return RawDiskSpec{}, fmt.Errorf("invalid raw disk uuid %q: %w", value, err)
			}
			spec.UUID = value
		case "version":
			spec.Version = value
		case "mnt":
			cleanMount := gopath.Clean(value)
			if !gopath.IsAbs(cleanMount) {
				return RawDiskSpec{}, fmt.Errorf("raw disk mount target must be an absolute guest path, got %q", value)
			}
			spec.MountTo = cleanMount
		default:
			return RawDiskSpec{}, fmt.Errorf("unsupported raw disk option %q", key)
		}
	}

	return spec, nil
}

func (v *machineBuilder) prepareRawDisk(ctx context.Context, spec RawDiskSpec) (define.BlkDev, error) {
	spec, err := normalizeRawDiskSpec(spec)
	if err != nil {
		return define.BlkDev{}, err
	}

	exists, err := rawDiskExists(spec.Path)
	if err != nil {
		return define.BlkDev{}, err
	}

	if exists {
		recreate, err := shouldRecreateRAWDisk(ctx, spec)
		if err != nil {
			return define.BlkDev{}, err
		}
		if recreate {
			if err := os.Remove(spec.Path); err != nil && !os.IsNotExist(err) {
				return define.BlkDev{}, fmt.Errorf("remove stale raw disk %q: %w", spec.Path, err)
			}
			return createRAWDisk(ctx, spec)
		}

		return inspectRAWDisk(ctx, spec.Path, spec.MountTo)
	}

	return createRAWDisk(ctx, spec)
}

func normalizeRawDiskSpec(spec RawDiskSpec) (RawDiskSpec, error) {
	spec.Path = strings.TrimSpace(spec.Path)
	if spec.Path == "" {
		return RawDiskSpec{}, fmt.Errorf("raw disk path cannot be empty")
	}

	absPath, err := filepath.Abs(filepath.Clean(spec.Path))
	if err != nil {
		return RawDiskSpec{}, err
	}
	spec.Path = absPath

	spec.UUID = strings.TrimSpace(spec.UUID)
	if spec.UUID != "" {
		if _, err := uuid.Parse(spec.UUID); err != nil {
			return RawDiskSpec{}, fmt.Errorf("invalid raw disk uuid %q: %w", spec.UUID, err)
		}
	}

	spec.Version = strings.TrimSpace(spec.Version)
	spec.MountTo = strings.TrimSpace(spec.MountTo)
	if spec.MountTo != "" {
		spec.MountTo = gopath.Clean(spec.MountTo)
		if !gopath.IsAbs(spec.MountTo) {
			return RawDiskSpec{}, fmt.Errorf("raw disk mount target must be an absolute guest path, got %q", spec.MountTo)
		}
	}

	return spec, nil
}

func rawDiskExists(rawDiskPath string) (bool, error) {
	_, err := os.Stat(rawDiskPath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("stat raw disk %q: %w", rawDiskPath, err)
}

func shouldRecreateRAWDisk(ctx context.Context, spec RawDiskSpec) (bool, error) {
	if spec.Version == "" {
		return false, nil
	}

	stored, ok, err := newRawDiskXattrManager().LookupXattr(ctx, spec.Path, define.XattrDiskVersionKey)
	if err != nil {
		return false, fmt.Errorf("read raw disk version xattr from %q: %w", spec.Path, err)
	}
	if !ok {
		return false, nil
	}

	return stored != spec.Version, nil
}

func createRAWDisk(ctx context.Context, spec RawDiskSpec) (define.BlkDev, error) {
	diskUUID := spec.UUID
	if diskUUID == "" {
		diskUUID = uuid.NewString()
	}

	diskMgr, err := newRawDiskManager()
	if err != nil {
		return define.BlkDev{}, err
	}

	if err := extractEmbeddedRAWDisk(ctx, spec.Path); err != nil {
		return define.BlkDev{}, fmt.Errorf("extract embedded raw disk to %q: %w", spec.Path, err)
	}

	if err := diskMgr.NewUUID(ctx, diskUUID, spec.Path); err != nil {
		return define.BlkDev{}, fmt.Errorf("write uuid %q to raw disk %q: %w", diskUUID, spec.Path, err)
	}

	if spec.Version != "" {
		if err := newRawDiskXattrManager().SetXattr(ctx, spec.Path, define.XattrDiskVersionKey, spec.Version, true); err != nil {
			return define.BlkDev{}, fmt.Errorf("write raw disk version xattr on %q: %w", spec.Path, err)
		}
	}

	return inspectRAWDisk(ctx, spec.Path, spec.MountTo)
}

func inspectRAWDisk(ctx context.Context, rawDiskPath string, mountOverride string) (define.BlkDev, error) {
	diskMgr, err := newRawDiskManager()
	if err != nil {
		return define.BlkDev{}, err
	}

	info, err := diskMgr.Inspect(ctx, rawDiskPath)
	if err != nil {
		return define.BlkDev{}, err
	}

	info.MountTo = resolveRawDiskMount(info.UUID, mountOverride)
	return *info, nil
}

func resolveRawDiskMount(diskUUID string, mountOverride string) string {
	if mountOverride != "" {
		return mountOverride
	}
	return fmt.Sprintf("/mnt/%s", diskUUID)
}
