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
	"github.com/sirupsen/logrus"
)

type RawDiskSpec struct {
	Path    string `json:"path,omitempty"`
	UUID    string `json:"uuid,omitempty"`
	Version string `json:"version,omitempty"`
	MountTo string `json:"mountTo,omitempty"`
}

type ContainerDiskSpec struct {
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
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

func ParseContainerDiskSpec(input string) (ContainerDiskSpec, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return ContainerDiskSpec{}, fmt.Errorf("container disk spec cannot be empty")
	}

	parts := strings.Split(input, ",")
	spec := ContainerDiskSpec{
		Path: strings.TrimSpace(parts[0]),
	}
	if spec.Path == "" {
		return ContainerDiskSpec{}, fmt.Errorf("container disk path cannot be empty")
	}

	seen := map[string]struct{}{}
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if part == "" {
			return ContainerDiskSpec{}, fmt.Errorf("container disk spec %q contains an empty option", input)
		}

		key, value, ok := strings.Cut(part, "=")
		if !ok {
			return ContainerDiskSpec{}, fmt.Errorf("container disk option %q must use key=value syntax", part)
		}

		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			return ContainerDiskSpec{}, fmt.Errorf("container disk option %q must have non-empty key and value", part)
		}
		if _, exists := seen[key]; exists {
			return ContainerDiskSpec{}, fmt.Errorf("container disk option %q is duplicated", key)
		}
		seen[key] = struct{}{}

		switch key {
		case "version":
			spec.Version = value
		default:
			return ContainerDiskSpec{}, fmt.Errorf("unsupported container disk option %q", key)
		}
	}

	return spec, nil
}

func (v *machineBuilder) prepareRawDisk(ctx context.Context, spec RawDiskSpec) (define.BlkDev, error) {
	spec, err := normalizeRawDiskSpec(spec)
	if err != nil {
		return define.BlkDev{}, err
	}

	logrus.Infof("preparing raw disk: path=%q requested_uuid=%q requested_version=%q requested_mount=%q", spec.Path, spec.UUID, spec.Version, spec.MountTo)

	exists, err := rawDiskExists(spec.Path)
	if err != nil {
		return define.BlkDev{}, err
	}

	if exists {
		logrus.Infof("raw disk already exists: path=%q", spec.Path)

		recreate, err := shouldRecreateRAWDisk(ctx, spec)
		if err != nil {
			return define.BlkDev{}, err
		}
		if recreate {
			logrus.Infof("recreating raw disk: path=%q", spec.Path)
			if err := os.Remove(spec.Path); err != nil && !os.IsNotExist(err) {
				return define.BlkDev{}, fmt.Errorf("remove stale raw disk %q: %w", spec.Path, err)
			}
			return createRAWDisk(ctx, spec)
		}

		if spec.UUID != "" {
			logrus.Infof("raw disk exists, requested uuid is ignored: path=%q requested_uuid=%q", spec.Path, spec.UUID)
		}

		return inspectRAWDisk(ctx, spec.Path, spec.MountTo)
	}

	return createRAWDisk(ctx, spec)
}

func (v *machineBuilder) prepareContainerStorageDisk(ctx context.Context, spec *ContainerDiskSpec, defaultPath string) (define.BlkDev, error) {
	rawDiskSpec, err := resolveContainerDiskSpec(spec, defaultPath)
	if err != nil {
		return define.BlkDev{}, err
	}

	logrus.Infof("preparing container disk: path=%q requested_version=%q effective_version=%q mount=%q", rawDiskSpec.Path, containerDiskVersionValue(spec), rawDiskSpec.Version, rawDiskSpec.MountTo)

	exists, err := rawDiskExists(rawDiskSpec.Path)
	if err != nil {
		return define.BlkDev{}, err
	}

	if exists {
		logrus.Infof("container disk already exists: path=%q", rawDiskSpec.Path)

		recreate, err := shouldBumpContainerDisk(ctx, rawDiskSpec)
		if err != nil {
			return define.BlkDev{}, err
		}
		if recreate {
			logrus.Infof("recreating container disk: path=%q", rawDiskSpec.Path)
			if err := os.Remove(rawDiskSpec.Path); err != nil && !os.IsNotExist(err) {
				return define.BlkDev{}, fmt.Errorf("remove stale container disk %q: %w", rawDiskSpec.Path, err)
			}
			return createRAWDisk(ctx, rawDiskSpec)
		}

		return inspectRAWDisk(ctx, rawDiskSpec.Path, rawDiskSpec.MountTo)
	}

	return createRAWDisk(ctx, rawDiskSpec)
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

func resolveContainerDiskSpec(spec *ContainerDiskSpec, defaultPath string) (RawDiskSpec, error) {
	resolved := ContainerDiskSpec{
		Path:    defaultPath,
		Version: define.DefaultContainerDiskVersion,
	}
	if spec != nil {
		if strings.TrimSpace(spec.Path) != "" {
			resolved.Path = spec.Path
		}
		if strings.TrimSpace(spec.Version) != "" {
			resolved.Version = spec.Version
		}
	}

	return normalizeRawDiskSpec(RawDiskSpec{
		Path:    resolved.Path,
		UUID:    define.ContainerDiskUUID,
		Version: resolved.Version,
		MountTo: define.ContainerStorageMountPoint,
	})
}

func containerDiskVersionValue(spec *ContainerDiskSpec) string {
	if spec == nil {
		return ""
	}
	return spec.Version
}

func normalizeDiskVersionValue(value string) string {
	return strings.TrimRight(value, "\r\n\x00")
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
		logrus.Infof("raw disk version not requested, skipping xattr comparison: path=%q", spec.Path)
		return false, nil
	}

	logrus.Infof("checking raw disk version xattr: path=%q key=%q expected=%q", spec.Path, define.XattrDiskVersionKey, spec.Version)
	stored, ok, err := newRawDiskXattrManager().LookupXattr(ctx, spec.Path, define.XattrDiskVersionKey)
	if err != nil {
		return false, fmt.Errorf("read raw disk version xattr from %q: %w", spec.Path, err)
	}
	if !ok {
		logrus.Infof("raw disk version xattr is missing, keeping existing disk: path=%q", spec.Path)
		return false, nil
	}

	normalizedStored := normalizeDiskVersionValue(stored)
	normalizedExpected := normalizeDiskVersionValue(spec.Version)
	if normalizedStored != normalizedExpected {
		logrus.Infof("raw disk version mismatch: path=%q stored=%q normalized_stored=%q expected=%q", spec.Path, stored, normalizedStored, spec.Version)
		return true, nil
	}

	logrus.Infof("raw disk version matches: path=%q stored=%q normalized=%q", spec.Path, stored, normalizedStored)
	return false, nil
}

func shouldBumpContainerDisk(ctx context.Context, spec RawDiskSpec) (bool, error) {
	logrus.Infof("checking container disk version xattr: path=%q key=%q expected=%q", spec.Path, define.XattrDiskVersionKey, spec.Version)
	stored, ok, err := newRawDiskXattrManager().LookupXattr(ctx, spec.Path, define.XattrDiskVersionKey)
	if err != nil {
		return false, fmt.Errorf("read container disk version xattr from %q: %w", spec.Path, err)
	}
	if !ok {
		logrus.Infof("container disk version xattr is missing, bumping disk: path=%q", spec.Path)
		return true, nil
	}
	normalizedStored := normalizeDiskVersionValue(stored)
	normalizedExpected := normalizeDiskVersionValue(spec.Version)
	if normalizedStored != normalizedExpected {
		logrus.Infof("container disk version mismatch: path=%q stored=%q normalized_stored=%q expected=%q", spec.Path, stored, normalizedStored, spec.Version)
		return true, nil
	}

	logrus.Infof("container disk version matches: path=%q stored=%q normalized=%q", spec.Path, stored, normalizedStored)
	return false, nil
}

func createRAWDisk(ctx context.Context, spec RawDiskSpec) (define.BlkDev, error) {
	diskUUID := spec.UUID
	if diskUUID == "" {
		diskUUID = uuid.NewString()
	}

	logrus.Infof("creating raw disk: path=%q uuid=%q mount=%q", spec.Path, diskUUID, resolveRawDiskMount(diskUUID, spec.MountTo))

	diskMgr, err := newRawDiskManager()
	if err != nil {
		return define.BlkDev{}, err
	}

	logrus.Infof("extracting embedded raw disk image: path=%q", spec.Path)
	if err := extractEmbeddedRAWDisk(ctx, spec.Path); err != nil {
		return define.BlkDev{}, fmt.Errorf("extract embedded raw disk to %q: %w", spec.Path, err)
	}

	logrus.Infof("writing raw disk uuid: path=%q uuid=%q", spec.Path, diskUUID)
	if err := diskMgr.NewUUID(ctx, diskUUID, spec.Path); err != nil {
		return define.BlkDev{}, fmt.Errorf("write uuid %q to raw disk %q: %w", diskUUID, spec.Path, err)
	}

	if spec.Version != "" {
		logrus.Infof("writing raw disk version xattr: path=%q key=%q value=%q", spec.Path, define.XattrDiskVersionKey, spec.Version)
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
	logrus.Infof("raw disk ready: path=%q uuid=%q mount=%q fstype=%q", info.Path, info.UUID, info.MountTo, info.FsType)
	return *info, nil
}

func resolveRawDiskMount(diskUUID string, mountOverride string) string {
	if mountOverride != "" {
		return mountOverride
	}
	return fmt.Sprintf("/mnt/%s", diskUUID)
}
