package revm

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

func ListVolumes() ([]Volume, error) {
	r, err := ResolveRoots()
	if err != nil {
		return nil, err
	}
	return volumes(r)
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
	catalog, err := acquireLock(filepath.Join(r.Data, "locks", "catalog.lock"))
	if err != nil {
		return Volume{}, err
	}
	defer catalog.Close()
	list, err := volumes(r)
	if err != nil {
		return Volume{}, err
	}
	for _, v := range list {
		if v.Name == name {
			return Volume{}, fmt.Errorf("volume %q already exists", name)
		}
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
	if err = os.Rename(temp, r.volumeDir(v.ID)); err != nil {
		return v, err
	}
	return v, syncDir(base)
}
func findVolume(r Roots, name string) (Volume, error) {
	list, err := volumes(r)
	if err != nil {
		return Volume{}, err
	}
	for _, v := range list {
		if v.Name == name {
			return v, nil
		}
	}
	return Volume{}, fmt.Errorf("volume %q not found; create it with revm volume create", name)
}
func RemoveVolume(name string) error {
	r, err := ResolveRoots()
	if err != nil {
		return err
	}
	catalog, err := acquireLock(filepath.Join(r.Data, "locks", "catalog.lock"))
	if err != nil {
		return err
	}
	defer catalog.Close()
	list, err := volumes(r)
	if err != nil {
		return err
	}
	for _, v := range list {
		if v.Name != name {
			continue
		}
		lock, err := acquireLock(r.volumeLock(v.ID))
		if err != nil {
			return err
		}
		defer lock.Close()
		diskLock, err := lockDisk(filepath.Join(r.volumeDir(v.ID), "disk.ext4"))
		if err != nil {
			return err
		}
		defer diskLock.Close()
		return os.RemoveAll(r.volumeDir(v.ID))
	}
	return fmt.Errorf("volume %q not found", name)
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
