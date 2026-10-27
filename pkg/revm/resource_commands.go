package revm

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

type VolumeView struct {
	Volume
	Path           string `json:"path"`
	SizeBytes      *int64 `json:"sizeBytes"`
	AllocatedBytes *int64 `json:"allocatedBytes"`
	InUse          bool   `json:"inUse"`
	Error          string `json:"error,omitempty"`
}

func ListVolumes() ([]VolumeView, error) {
	r, err := ResolveRoots()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(r.Data, "volumes"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	views := []VolumeView{}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		name := entry.Name()
		view := VolumeView{Volume: Volume{Name: name}, Path: filepath.Join(r.volumeDir(name), "disk.ext4")}
		v, err := loadVolume(r, name)
		if err != nil {
			view.Error = err.Error()
			views = append(views, view)
			continue
		}
		view.Volume = v
		var stat unix.Stat_t
		statErr := unix.Stat(view.Path, &stat)
		if statErr == nil {
			allocated := stat.Blocks * 512
			view.SizeBytes, view.AllocatedBytes = &stat.Size, &allocated
		}
		active, lockErr := activeLock(r.volumeLock(name))
		diskLock, diskErr := lockDisk(view.Path)
		if diskLock != nil {
			diskLock.Close()
		}
		view.InUse = active || errors.Is(diskErr, unix.EWOULDBLOCK)
		if errors.Is(diskErr, unix.EWOULDBLOCK) {
			diskErr = nil
		}
		if err := errors.Join(statErr, lockErr, diskErr); err != nil {
			view.Error = err.Error()
		}
		views = append(views, view)
	}
	return views, nil
}

// CreateVolume creates independent persistent storage. It never replaces data.
func CreateVolume(ctx context.Context, name string) (Volume, error) {
	if err := validateSessionID(name); err != nil {
		return Volume{}, err
	}
	r, err := ResolveRoots()
	if err != nil {
		return Volume{}, err
	}
	lock, err := acquireLock(r.volumeLock(name))
	if err != nil {
		return Volume{}, err
	}
	defer lock.Close()
	if _, err := os.Lstat(r.volumeDir(name)); err == nil {
		return Volume{}, fmt.Errorf("volume %q already exists", name)
	} else if !os.IsNotExist(err) {
		return Volume{}, err
	}
	v := Volume{Version: 1, ID: uuid.NewString(), Name: name, Purpose: "podman", Format: "ext4"}
	base := filepath.Join(r.Data, "volumes")
	if err = privateDir(base); err != nil {
		return v, err
	}
	temp, err := os.MkdirTemp(base, ".create-")
	if err != nil {
		return v, err
	}
	defer os.RemoveAll(temp)
	spec, err := resolveContainerDiskSpec(nil, filepath.Join(temp, "disk.ext4"))
	if err != nil {
		return v, err
	}
	if _, err = createRAWDisk(ctx, spec); err != nil {
		return v, err
	}
	if err = writeJSONAtomic(filepath.Join(temp, "volume.json"), v); err != nil {
		return v, err
	}
	if err = os.Rename(temp, r.volumeDir(name)); err != nil {
		return v, err
	}
	return v, syncDir(base)
}
func RemoveVolume(name string) error {
	if err := validateSessionID(name); err != nil {
		return err
	}
	r, err := ResolveRoots()
	if err != nil {
		return err
	}
	lock, err := acquireLock(r.volumeLock(name))
	if err != nil {
		return err
	}
	defer lock.Close()
	path := r.volumeDir(name)
	if _, err := os.Lstat(path); err != nil {
		return fmt.Errorf("volume %q: %w", name, err)
	}
	diskLock, err := lockDisk(filepath.Join(path, "disk.ext4"))
	if err != nil {
		return err
	}
	defer diskLock.Close()
	return os.RemoveAll(path)
}
func lockDisk(path string) (*os.File, error) {
	canonical, err := canonicalPath(path)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(canonical))
	return acquireLock(filepath.Join(runtimeFallback(), "locks", fmt.Sprintf("disk-%x.lock", sum)))
}

// PruneCache only recognizes instance directories with a valid ownership
// record. Legacy sessions and arbitrary files below Cache are never traversed.
func PruneCache() ([]string, error) {
	r, err := ResolveRoots()
	if err != nil {
		return nil, err
	}
	removed := []string{}
	for _, base := range []string{filepath.Join(r.Cache, "instances"), r.Runtime} {
		entries, err := os.ReadDir(base)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return removed, err
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			path := filepath.Join(base, e.Name())
			var owner RunRecord
			if readJSON(filepath.Join(path, "owner.json"), &owner) != nil || owner.Version != 1 || owner.StateRoot != r.State {
				continue
			}
			if validateSessionID(owner.SessionID) != nil {
				continue
			}
			if _, err := uuid.Parse(owner.InstanceID); err != nil {
				continue
			}
			if path != owner.RuntimeDir && path != owner.CacheDir {
				continue
			}
			lock, err := acquireLock(r.sessionLock(owner.SessionID))
			if errors.Is(err, unix.EWOULDBLOCK) {
				continue
			}
			if err != nil {
				return removed, err
			}
			err = removeOwned(path, owner.InstanceID)
			lock.Close()
			if err != nil {
				return removed, err
			}
			removed = append(removed, path)
		}
	}
	return removed, nil
}
