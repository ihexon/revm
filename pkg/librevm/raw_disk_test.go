//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/disk"
	"linuxvm/pkg/filesystem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type fakeRawDiskManager struct {
	uuids        map[string]string
	newUUIDCalls []fakeNewUUIDCall
}

type fakeNewUUIDCall struct {
	path string
	uuid string
}

func (f *fakeRawDiskManager) Inspect(_ context.Context, blkPath string) (*define.BlkDev, error) {
	blkPath = mustAbsPath(blkPath)
	diskUUID, ok := f.uuids[blkPath]
	if !ok {
		return nil, fmt.Errorf("unknown fake disk %q", blkPath)
	}

	return &define.BlkDev{
		UUID:   diskUUID,
		FsType: "ext4",
		Path:   blkPath,
	}, nil
}

func (f *fakeRawDiskManager) Create(context.Context, string, uint64) error {
	return nil
}

func (f *fakeRawDiskManager) NewUUID(_ context.Context, id string, blkPath string) error {
	blkPath = mustAbsPath(blkPath)
	f.uuids[blkPath] = id
	f.newUUIDCalls = append(f.newUUIDCalls, fakeNewUUIDCall{
		path: blkPath,
		uuid: id,
	})
	return nil
}

type fakeXattrManager struct {
	values map[string]map[string]string
}

func (f *fakeXattrManager) SetXattr(_ context.Context, filePath string, key string, value string, overwrite bool) error {
	filePath = mustAbsPath(filePath)
	if f.values[filePath] == nil {
		f.values[filePath] = map[string]string{}
	}
	if _, exists := f.values[filePath][key]; exists && !overwrite {
		return nil
	}
	f.values[filePath][key] = value
	return nil
}

func (f *fakeXattrManager) GetXattr(_ context.Context, filePath string, key string) (string, error) {
	value, ok, err := f.LookupXattr(context.Background(), filePath, key)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("missing xattr %q on %q", key, filePath)
	}
	return value, nil
}

func (f *fakeXattrManager) LookupXattr(_ context.Context, filePath string, key string) (string, bool, error) {
	filePath = mustAbsPath(filePath)
	value, ok := f.values[filePath][key]
	return value, ok, nil
}

func TestParseRawDiskSpec_PathOnly(t *testing.T) {
	spec, err := ParseRawDiskSpec("/tmp/data.ext4")
	if err != nil {
		t.Fatalf("ParseRawDiskSpec returned error: %v", err)
	}

	if spec.Path != "/tmp/data.ext4" {
		t.Fatalf("unexpected path: %q", spec.Path)
	}
	if spec.UUID != "" || spec.Version != "" || spec.MountTo != "" {
		t.Fatalf("expected only path to be set, got %+v", spec)
	}
}

func TestParseRawDiskSpec_WithOptionalFields(t *testing.T) {
	diskUUID := uuid.NewString()
	spec, err := ParseRawDiskSpec("/tmp/data.ext4,uuid=" + diskUUID + ",version=v2,mnt=/workspace/data")
	if err != nil {
		t.Fatalf("ParseRawDiskSpec returned error: %v", err)
	}

	if spec.UUID != diskUUID {
		t.Fatalf("unexpected uuid: %q", spec.UUID)
	}
	if spec.Version != "v2" {
		t.Fatalf("unexpected version: %q", spec.Version)
	}
	if spec.MountTo != "/workspace/data" {
		t.Fatalf("unexpected mount target: %q", spec.MountTo)
	}
}

func TestParseRawDiskSpec_RejectsNonKeyValueOptions(t *testing.T) {
	_, err := ParseRawDiskSpec("/tmp/data.ext4," + uuid.NewString())
	if err == nil {
		t.Fatal("expected ParseRawDiskSpec to reject bare option")
	}
	if !strings.Contains(err.Error(), "key=value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseContainerDiskSpec_PathOnly(t *testing.T) {
	spec, err := ParseContainerDiskSpec("/tmp/container-storage.ext4")
	if err != nil {
		t.Fatalf("ParseContainerDiskSpec returned error: %v", err)
	}

	if spec.Path != "/tmp/container-storage.ext4" {
		t.Fatalf("unexpected path: %q", spec.Path)
	}
	if spec.Version != "" {
		t.Fatalf("expected version to be empty, got %q", spec.Version)
	}
}

func TestParseContainerDiskSpec_WithVersion(t *testing.T) {
	spec, err := ParseContainerDiskSpec("/tmp/container-storage.ext4,version=v2")
	if err != nil {
		t.Fatalf("ParseContainerDiskSpec returned error: %v", err)
	}

	if spec.Path != "/tmp/container-storage.ext4" {
		t.Fatalf("unexpected path: %q", spec.Path)
	}
	if spec.Version != "v2" {
		t.Fatalf("unexpected version: %q", spec.Version)
	}
}

func TestParseContainerDiskSpec_RejectsUnsupportedOption(t *testing.T) {
	_, err := ParseContainerDiskSpec("/tmp/container-storage.ext4,uuid=" + uuid.NewString())
	if err == nil {
		t.Fatal("expected ParseContainerDiskSpec to reject unsupported option")
	}
	if !strings.Contains(err.Error(), "unsupported container disk option") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureRAWDisk_CreatesMissingDiskWithDefaults(t *testing.T) {
	diskMgr, xattrMgr, extracted := installRawDiskTestDoubles(t)
	rawDiskPath := filepath.Join(t.TempDir(), "new.ext4")

	blkDev, err := (&machineBuilder{}).prepareRawDisk(context.Background(), RawDiskSpec{
		Path:    rawDiskPath,
		Version: "v1",
	})
	if err != nil {
		t.Fatalf("prepareRawDisk returned error: %v", err)
	}

	rawDiskPath = mustAbsPath(rawDiskPath)
	if len(*extracted) != 1 || (*extracted)[0] != rawDiskPath {
		t.Fatalf("expected raw disk to be extracted once, got %v", *extracted)
	}
	if len(diskMgr.newUUIDCalls) != 1 {
		t.Fatalf("expected one UUID write, got %d", len(diskMgr.newUUIDCalls))
	}
	if blkDev.UUID == "" {
		t.Fatal("expected generated UUID to be set")
	}
	if blkDev.MountTo != "/mnt/"+blkDev.UUID {
		t.Fatalf("unexpected mount target: %q", blkDev.MountTo)
	}
	if got := xattrMgr.values[rawDiskPath][define.XattrDiskVersionKey]; got != "v1" {
		t.Fatalf("unexpected version xattr: %q", got)
	}
}

func TestEnsureRAWDisk_ExistingDiskIgnoresUUIDButAppliesMount(t *testing.T) {
	diskMgr, _, extracted := installRawDiskTestDoubles(t)
	rawDiskPath := filepath.Join(t.TempDir(), "existing.ext4")
	createTestFile(t, rawDiskPath)

	rawDiskPath = mustAbsPath(rawDiskPath)
	existingUUID := uuid.NewString()
	diskMgr.uuids[rawDiskPath] = existingUUID

	blkDev, err := (&machineBuilder{}).prepareRawDisk(context.Background(), RawDiskSpec{
		Path:    rawDiskPath,
		UUID:    uuid.NewString(),
		MountTo: "/guest/data",
	})
	if err != nil {
		t.Fatalf("prepareRawDisk returned error: %v", err)
	}

	if len(*extracted) != 0 {
		t.Fatalf("expected existing disk to be reused, got extractions %v", *extracted)
	}
	if len(diskMgr.newUUIDCalls) != 0 {
		t.Fatalf("expected existing disk UUID to stay untouched, got %v", diskMgr.newUUIDCalls)
	}
	if blkDev.UUID != existingUUID {
		t.Fatalf("expected existing UUID %q, got %q", existingUUID, blkDev.UUID)
	}
	if blkDev.MountTo != "/guest/data" {
		t.Fatalf("unexpected mount target: %q", blkDev.MountTo)
	}
}

func TestEnsureRAWDisk_ExistingDiskWithoutVersionXattrDoesNotRecreate(t *testing.T) {
	diskMgr, xattrMgr, extracted := installRawDiskTestDoubles(t)
	rawDiskPath := filepath.Join(t.TempDir(), "existing-no-version.ext4")
	createTestFile(t, rawDiskPath)

	rawDiskPath = mustAbsPath(rawDiskPath)
	existingUUID := uuid.NewString()
	diskMgr.uuids[rawDiskPath] = existingUUID

	blkDev, err := (&machineBuilder{}).prepareRawDisk(context.Background(), RawDiskSpec{
		Path:    rawDiskPath,
		Version: "v2",
	})
	if err != nil {
		t.Fatalf("prepareRawDisk returned error: %v", err)
	}

	if len(*extracted) != 0 {
		t.Fatalf("expected existing disk to be reused, got extractions %v", *extracted)
	}
	if len(diskMgr.newUUIDCalls) != 0 {
		t.Fatalf("expected no UUID rewrite, got %v", diskMgr.newUUIDCalls)
	}
	if _, ok := xattrMgr.values[rawDiskPath][define.XattrDiskVersionKey]; ok {
		t.Fatalf("expected missing version xattr to stay missing")
	}
	if blkDev.UUID != existingUUID {
		t.Fatalf("expected existing UUID %q, got %q", existingUUID, blkDev.UUID)
	}
}

func TestEnsureRAWDisk_RecreatesWhenVersionMismatches(t *testing.T) {
	diskMgr, xattrMgr, extracted := installRawDiskTestDoubles(t)
	rawDiskPath := filepath.Join(t.TempDir(), "existing-mismatch.ext4")
	createTestFile(t, rawDiskPath)

	rawDiskPath = mustAbsPath(rawDiskPath)
	diskMgr.uuids[rawDiskPath] = uuid.NewString()
	xattrMgr.values[rawDiskPath] = map[string]string{
		define.XattrDiskVersionKey: "old-version",
	}

	newUUID := uuid.NewString()
	blkDev, err := (&machineBuilder{}).prepareRawDisk(context.Background(), RawDiskSpec{
		Path:    rawDiskPath,
		UUID:    newUUID,
		Version: "new-version",
	})
	if err != nil {
		t.Fatalf("prepareRawDisk returned error: %v", err)
	}

	if len(*extracted) != 1 || (*extracted)[0] != rawDiskPath {
		t.Fatalf("expected disk recreation, got extractions %v", *extracted)
	}
	if len(diskMgr.newUUIDCalls) != 1 || diskMgr.newUUIDCalls[0].uuid != newUUID {
		t.Fatalf("expected recreated disk UUID %q, got %v", newUUID, diskMgr.newUUIDCalls)
	}
	if got := xattrMgr.values[rawDiskPath][define.XattrDiskVersionKey]; got != "new-version" {
		t.Fatalf("unexpected version xattr after recreation: %q", got)
	}
	if blkDev.UUID != newUUID {
		t.Fatalf("expected recreated UUID %q, got %q", newUUID, blkDev.UUID)
	}
	if blkDev.MountTo != "/mnt/"+newUUID {
		t.Fatalf("unexpected mount target: %q", blkDev.MountTo)
	}
}

func TestPrepareContainerStorageDisk_DefaultsWhenUnset(t *testing.T) {
	diskMgr, xattrMgr, extracted := installRawDiskTestDoubles(t)
	defaultPath := filepath.Join(t.TempDir(), "container-storage.ext4")

	blkDev, err := (&machineBuilder{}).prepareContainerStorageDisk(context.Background(), nil, defaultPath)
	if err != nil {
		t.Fatalf("prepareContainerStorageDisk returned error: %v", err)
	}

	defaultPath = mustAbsPath(defaultPath)
	if len(*extracted) != 1 || (*extracted)[0] != defaultPath {
		t.Fatalf("expected container disk to be extracted once, got %v", *extracted)
	}
	if len(diskMgr.newUUIDCalls) != 1 || diskMgr.newUUIDCalls[0].uuid != define.ContainerDiskUUID {
		t.Fatalf("expected container disk UUID %q, got %v", define.ContainerDiskUUID, diskMgr.newUUIDCalls)
	}
	if got := xattrMgr.values[defaultPath][define.XattrDiskVersionKey]; got != define.DefaultContainerDiskVersion {
		t.Fatalf("unexpected default container disk version xattr: %q", got)
	}
	if blkDev.MountTo != define.ContainerStorageMountPoint {
		t.Fatalf("unexpected mount target: %q", blkDev.MountTo)
	}
}

func TestPrepareContainerStorageDisk_RecreatesWhenVersionXattrMissing(t *testing.T) {
	diskMgr, xattrMgr, extracted := installRawDiskTestDoubles(t)
	rawDiskPath := filepath.Join(t.TempDir(), "container-storage.ext4")
	createTestFile(t, rawDiskPath)

	rawDiskPath = mustAbsPath(rawDiskPath)
	diskMgr.uuids[rawDiskPath] = uuid.NewString()

	blkDev, err := (&machineBuilder{}).prepareContainerStorageDisk(context.Background(), &ContainerDiskSpec{
		Path:    rawDiskPath,
		Version: "v2",
	}, filepath.Join(t.TempDir(), "unused.ext4"))
	if err != nil {
		t.Fatalf("prepareContainerStorageDisk returned error: %v", err)
	}

	if len(*extracted) != 1 || (*extracted)[0] != rawDiskPath {
		t.Fatalf("expected container disk recreation, got %v", *extracted)
	}
	if len(diskMgr.newUUIDCalls) != 1 || diskMgr.newUUIDCalls[0].uuid != define.ContainerDiskUUID {
		t.Fatalf("expected recreated container disk UUID %q, got %v", define.ContainerDiskUUID, diskMgr.newUUIDCalls)
	}
	if got := xattrMgr.values[rawDiskPath][define.XattrDiskVersionKey]; got != "v2" {
		t.Fatalf("unexpected container disk version xattr: %q", got)
	}
	if blkDev.MountTo != define.ContainerStorageMountPoint {
		t.Fatalf("unexpected mount target: %q", blkDev.MountTo)
	}
}

func TestPrepareContainerStorageDisk_RecreatesWhenVersionMismatches(t *testing.T) {
	diskMgr, xattrMgr, extracted := installRawDiskTestDoubles(t)
	rawDiskPath := filepath.Join(t.TempDir(), "container-storage.ext4")
	createTestFile(t, rawDiskPath)

	rawDiskPath = mustAbsPath(rawDiskPath)
	diskMgr.uuids[rawDiskPath] = define.ContainerDiskUUID
	xattrMgr.values[rawDiskPath] = map[string]string{
		define.XattrDiskVersionKey: "old-version",
	}

	_, err := (&machineBuilder{}).prepareContainerStorageDisk(context.Background(), &ContainerDiskSpec{
		Path:    rawDiskPath,
		Version: "new-version",
	}, filepath.Join(t.TempDir(), "unused.ext4"))
	if err != nil {
		t.Fatalf("prepareContainerStorageDisk returned error: %v", err)
	}

	if len(*extracted) != 1 || (*extracted)[0] != rawDiskPath {
		t.Fatalf("expected container disk recreation, got %v", *extracted)
	}
	if len(diskMgr.newUUIDCalls) != 1 || diskMgr.newUUIDCalls[0].uuid != define.ContainerDiskUUID {
		t.Fatalf("expected recreated container disk UUID %q, got %v", define.ContainerDiskUUID, diskMgr.newUUIDCalls)
	}
	if got := xattrMgr.values[rawDiskPath][define.XattrDiskVersionKey]; got != "new-version" {
		t.Fatalf("unexpected container disk version xattr: %q", got)
	}
}

func TestPrepareContainerStorageDisk_ReusesWhenVersionMatches(t *testing.T) {
	diskMgr, xattrMgr, extracted := installRawDiskTestDoubles(t)
	rawDiskPath := filepath.Join(t.TempDir(), "container-storage.ext4")
	createTestFile(t, rawDiskPath)

	rawDiskPath = mustAbsPath(rawDiskPath)
	diskMgr.uuids[rawDiskPath] = define.ContainerDiskUUID
	xattrMgr.values[rawDiskPath] = map[string]string{
		define.XattrDiskVersionKey: "v3",
	}

	blkDev, err := (&machineBuilder{}).prepareContainerStorageDisk(context.Background(), &ContainerDiskSpec{
		Path:    rawDiskPath,
		Version: "v3",
	}, filepath.Join(t.TempDir(), "unused.ext4"))
	if err != nil {
		t.Fatalf("prepareContainerStorageDisk returned error: %v", err)
	}

	if len(*extracted) != 0 {
		t.Fatalf("expected existing container disk to be reused, got %v", *extracted)
	}
	if len(diskMgr.newUUIDCalls) != 0 {
		t.Fatalf("expected no UUID rewrite, got %v", diskMgr.newUUIDCalls)
	}
	if blkDev.MountTo != define.ContainerStorageMountPoint {
		t.Fatalf("unexpected mount target: %q", blkDev.MountTo)
	}
}

func installRawDiskTestDoubles(t *testing.T) (*fakeRawDiskManager, *fakeXattrManager, *[]string) {
	t.Helper()

	oldDiskManager := newRawDiskManager
	oldXattrManager := newRawDiskXattrManager
	oldExtractor := extractEmbeddedRAWDisk

	diskMgr := &fakeRawDiskManager{
		uuids: map[string]string{},
	}
	xattrMgr := &fakeXattrManager{
		values: map[string]map[string]string{},
	}
	extracted := []string{}

	newRawDiskManager = func() (disk.Manager, error) {
		return diskMgr, nil
	}
	newRawDiskXattrManager = func() filesystem.XattrManager {
		return xattrMgr
	}
	extractEmbeddedRAWDisk = func(_ context.Context, targetPath string) error {
		targetPath = mustAbsPath(targetPath)
		extracted = append(extracted, targetPath)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(targetPath, []byte("raw-disk"), 0o644)
	}

	t.Cleanup(func() {
		newRawDiskManager = oldDiskManager
		newRawDiskXattrManager = oldXattrManager
		extractEmbeddedRAWDisk = oldExtractor
	})

	return diskMgr, xattrMgr, &extracted
}

func createTestFile(t *testing.T, targetPath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("create test dir: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("raw-disk"), 0o644); err != nil {
		t.Fatalf("create test file: %v", err)
	}
}

func mustAbsPath(targetPath string) string {
	absPath, err := filepath.Abs(filepath.Clean(targetPath))
	if err != nil {
		panic(err)
	}
	return absPath
}
