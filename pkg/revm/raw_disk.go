//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/disk"
	"linuxvm/pkg/static_resources"
	"os"
	gopath "path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

type RawDiskSpec struct {
	Path     string `json:"path,omitempty"`
	UUID     string `json:"uuid,omitempty"`
	MountTo  string `json:"mountTo,omitempty"`
	ReadOnly bool   `json:"readOnly,omitempty"`
	DirectIO bool   `json:"directIO,omitempty"`
	SyncMode uint32 `json:"syncMode,omitempty"`
}

type ContainerDiskSpec struct {
	Path string `json:"path,omitempty"`
}

var (
	newRawDiskManager      = func() (disk.Manager, error) { return disk.NewBlkManager() }
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
		Path:     strings.TrimSpace(parts[0]),
		SyncMode: define.SyncModeRelaxed,
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
		case "mnt":
			cleanMount := gopath.Clean(value)
			if !gopath.IsAbs(cleanMount) {
				return RawDiskSpec{}, fmt.Errorf("raw disk mount target must be an absolute guest path, got %q", value)
			}
			spec.MountTo = cleanMount
		case "readonly", "ro":
			valueBool, err := strconv.ParseBool(value)
			if err != nil {
				return RawDiskSpec{}, fmt.Errorf("invalid raw disk readonly value %q", value)
			}
			spec.ReadOnly = valueBool
		case "directio", "direct-io":
			valueBool, err := strconv.ParseBool(value)
			if err != nil {
				return RawDiskSpec{}, fmt.Errorf("invalid raw disk directio value %q", value)
			}
			spec.DirectIO = valueBool
		case "sync":
			switch strings.ToLower(value) {
			case "none":
				spec.SyncMode = define.SyncModeNone
			case "relaxed":
				spec.SyncMode = define.SyncModeRelaxed
			case "full":
				spec.SyncMode = define.SyncModeFull
			default:
				return RawDiskSpec{}, fmt.Errorf("unsupported raw disk sync mode %q", value)
			}
		default:
			return RawDiskSpec{}, fmt.Errorf("unsupported raw disk option %q", key)
		}
	}

	return spec, nil
}

func ParseContainerDiskSpec(input string) (ContainerDiskSpec, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return ContainerDiskSpec{}, fmt.Errorf("container disk path cannot be empty")
	}
	return ContainerDiskSpec{Path: input}, nil
}

func (v *machineBuilder) prepareRawDisk(ctx context.Context, spec RawDiskSpec) (define.BlockDeviceSpec, error) {
	spec, err := normalizeRawDiskSpec(spec)
	if err != nil {
		return define.BlockDeviceSpec{}, err
	}

	logrus.Infof("preparing raw disk: path=%q requested_uuid=%q requested_mount=%q", spec.Path, spec.UUID, spec.MountTo)

	exists, err := rawDiskExists(spec.Path)
	if err != nil {
		return define.BlockDeviceSpec{}, err
	}

	if exists {
		logrus.Infof("raw disk already exists: path=%q", spec.Path)

		if spec.UUID != "" {
			logrus.Infof("raw disk exists, requested uuid is ignored: path=%q requested_uuid=%q", spec.Path, spec.UUID)
		}

		dev, err := inspectRAWDisk(ctx, spec.Path, spec.MountTo)
		return applyRawDiskRuntimeOptions(dev, spec), err
	}

	dev, err := createRAWDisk(ctx, spec)
	return applyRawDiskRuntimeOptions(dev, spec), err
}

func applyRawDiskRuntimeOptions(dev define.BlockDeviceSpec, spec RawDiskSpec) define.BlockDeviceSpec {
	if dev.GuestMount == nil {
		dev.GuestMount = &define.BlockMountSpec{}
	}
	dev.ID = dev.GuestMount.UUID
	dev.Format = define.DiskFormatRaw
	dev.ReadOnly = spec.ReadOnly
	dev.DirectIO = spec.DirectIO
	dev.SyncMode = spec.SyncMode
	dev.GuestMount.Target = resolveRawDiskMount(dev.GuestMount.UUID, spec.MountTo)
	dev.GuestMount.ReadOnly = spec.ReadOnly
	return dev
}

func (v *machineBuilder) prepareContainerStorageDisk(ctx context.Context, spec *ContainerDiskSpec, defaultPath string) (define.BlockDeviceSpec, error) {
	rawDiskSpec, err := resolveContainerDiskSpec(spec, defaultPath)
	if err != nil {
		return define.BlockDeviceSpec{}, err
	}

	return v.prepareRawDisk(ctx, rawDiskSpec)
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
	if spec != nil && strings.TrimSpace(spec.Path) != "" {
		defaultPath = spec.Path
	}
	return normalizeRawDiskSpec(RawDiskSpec{
		Path:     defaultPath,
		UUID:     define.ContainerDiskUUID,
		MountTo:  define.ContainerStorageMountPoint,
		SyncMode: define.SyncModeRelaxed,
	})
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

func createRAWDisk(ctx context.Context, spec RawDiskSpec) (define.BlockDeviceSpec, error) {
	target := spec.Path
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return define.BlockDeviceSpec{}, err
	}
	temp, err := os.MkdirTemp(filepath.Dir(target), ".disk-")
	if err != nil {
		return define.BlockDeviceSpec{}, err
	}
	defer os.RemoveAll(temp)
	spec.Path = filepath.Join(temp, "disk.ext4")

	diskUUID := spec.UUID
	if diskUUID == "" {
		diskUUID = uuid.NewString()
	}

	logrus.Infof("creating raw disk: path=%q uuid=%q mount=%q", spec.Path, diskUUID, resolveRawDiskMount(diskUUID, spec.MountTo))

	diskMgr, err := newRawDiskManager()
	if err != nil {
		return define.BlockDeviceSpec{}, err
	}

	logrus.Infof("extracting embedded raw disk image: path=%q", spec.Path)
	if err := extractEmbeddedRAWDisk(ctx, spec.Path); err != nil {
		return define.BlockDeviceSpec{}, fmt.Errorf("extract embedded raw disk to %q: %w", spec.Path, err)
	}

	logrus.Infof("writing raw disk uuid: path=%q uuid=%q", spec.Path, diskUUID)
	if err := diskMgr.NewUUID(ctx, diskUUID, spec.Path); err != nil {
		return define.BlockDeviceSpec{}, fmt.Errorf("write uuid %q to raw disk %q: %w", diskUUID, spec.Path, err)
	}

	if err := os.Link(spec.Path, target); err != nil {
		return define.BlockDeviceSpec{}, fmt.Errorf("publish new disk without replacing existing data: %w", err)
	}
	return inspectRAWDisk(ctx, target, spec.MountTo)
}

func inspectRAWDisk(ctx context.Context, rawDiskPath string, mountOverride string) (define.BlockDeviceSpec, error) {
	diskMgr, err := newRawDiskManager()
	if err != nil {
		return define.BlockDeviceSpec{}, err
	}

	info, err := diskMgr.Inspect(ctx, rawDiskPath)
	if err != nil {
		return define.BlockDeviceSpec{}, err
	}

	if info.GuestMount == nil {
		info.GuestMount = &define.BlockMountSpec{}
	}
	info.GuestMount.Target = resolveRawDiskMount(info.GuestMount.UUID, mountOverride)
	logrus.Infof("raw disk ready: path=%q uuid=%q mount=%q fstype=%q", info.Path, info.GuestMount.UUID, info.GuestMount.Target, info.GuestMount.FsType)
	return *info, nil
}

func resolveRawDiskMount(diskUUID string, mountOverride string) string {
	if mountOverride != "" {
		return mountOverride
	}
	return fmt.Sprintf("/mnt/%s", diskUUID)
}
