package revm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

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
		if v.Name != name && v.ID != name {
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

// MigrateSession is explicit and leaves the legacy source intact. A volume
// receipt makes a retry after publishing the volume but before the session safe.
func MigrateSession(ctx context.Context, id string) (Session, error) {
	var empty Session
	if err := validateSessionID(id); err != nil {
		return empty, err
	}
	r, err := ResolveRoots()
	if err != nil {
		return empty, err
	}
	lock, err := acquireLock(r.sessionLock(id))
	if err != nil {
		return empty, err
	}
	defer lock.Close()
	old := legacySessionDir(id)
	legacy, err := acquireLock(old + ".lock")
	if err != nil {
		return empty, err
	}
	defer legacy.Close()
	if _, err = os.Stat(old); err != nil {
		return empty, err
	}
	s, err := loadSession(r, id)
	if err == nil {
		return s, fmt.Errorf("session %q already has current metadata; legacy source retained at %s", id, old)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return empty, err
	}
	s = Session{Version: 1, ID: id, Config: Config{SessionID: id}}
	source := filepath.Join(old, "raw-disk", "container-storage.ext4")
	if _, err = os.Stat(source); err == nil {
		diskLock, err := lockDisk(source)
		if err != nil {
			return empty, err
		}
		defer diskLock.Close()
		if _, err := inspectRAWDisk(ctx, source, ""); err != nil {
			return empty, fmt.Errorf("inspect legacy disk (source preserved): %w", err)
		}
		catalog, err := acquireLock(filepath.Join(r.Data, "locks", "catalog.lock"))
		if err != nil {
			return empty, err
		}
		defer catalog.Close()
		list, err := volumes(r)
		if err != nil {
			return empty, err
		}
		var volume Volume
		for _, v := range list {
			if v.DefaultSession != id && v.Name != "podman-"+id {
				continue
			}
			var receipt struct {
				Source string `json:"source"`
			}
			if readJSON(filepath.Join(r.volumeDir(v.ID), "migration.json"), &receipt) != nil || receipt.Source != source {
				return empty, fmt.Errorf("migration conflicts with existing volume %s; both sources preserved", v.Name)
			}
			volume = v
		}
		if volume.ID == "" {
			volume = Volume{Version: 1, ID: uuid.NewString(), Name: "podman-" + id, DefaultSession: id, Purpose: "podman", Format: "ext4"}
			base := filepath.Join(r.Data, "volumes")
			if err := privateDir(base); err != nil {
				return empty, err
			}
			temp, err := os.MkdirTemp(base, ".migrate-")
			if err != nil {
				return empty, err
			}
			defer os.RemoveAll(temp)
			target := filepath.Join(temp, "disk.ext4")
			if err = copySparseVerified(ctx, source, target); err != nil {
				return empty, fmt.Errorf("copy legacy disk (source preserved): %w", err)
			}
			value, ok, err := newRawDiskXattrManager().LookupXattr(ctx, source, "user.vm.rawdisk.version")
			if err != nil {
				return empty, err
			}
			if ok {
				if err = newRawDiskXattrManager().SetXattr(ctx, target, "user.vm.rawdisk.version", normalizeDiskVersionValue(value), true); err != nil {
					return empty, err
				}
			}
			if err = writeJSONAtomic(filepath.Join(temp, "volume.json"), volume); err != nil {
				return empty, err
			}
			if err = writeJSONAtomic(filepath.Join(temp, "migration.json"), struct {
				Source string `json:"source"`
			}{source}); err != nil {
				return empty, err
			}
			if err = os.Rename(temp, r.volumeDir(volume.ID)); err != nil {
				return empty, err
			}
		}
		s.VolumeID = volume.ID
		s.Config.RunMode = ModeContainer
	} else if !os.IsNotExist(err) {
		return empty, err
	}
	// Copy only regular legacy log files, never follow links out of the source.
	entries, err := os.ReadDir(filepath.Join(old, "logs"))
	if err != nil && !os.IsNotExist(err) {
		return empty, err
	}
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(old, "logs", e.Name()))
		if err != nil {
			return empty, err
		}
		dest := filepath.Join(r.sessionDir(id), "legacy-logs", e.Name())
		if err = privateDir(filepath.Dir(dest)); err != nil {
			return empty, err
		}
		if err = os.WriteFile(dest, data, 0600); err != nil {
			return empty, err
		}
	}
	if err = writeJSONAtomic(filepath.Join(r.sessionDir(id), "session.json"), s); err != nil {
		return empty, err
	}
	return s, nil
}

// Copy allocated ranges and verify each against its source digest. New holes
// start zero-filled; copying a 500 GiB sparse disk never materializes its holes.
func copySparseVerified(ctx context.Context, src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	before, err := in.Stat()
	if err != nil {
		return err
	}
	if !before.Mode().IsRegular() {
		return fmt.Errorf("migration requires a regular disk file")
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	if err = out.Truncate(before.Size()); err != nil {
		return err
	}
	for pos := int64(0); pos < before.Size(); {
		if err := ctx.Err(); err != nil {
			return err
		}
		start, err := unix.Seek(int(in.Fd()), pos, unix.SEEK_DATA)
		if errors.Is(err, syscall.ENXIO) {
			break
		}
		if err != nil {
			return fmt.Errorf("filesystem does not support sparse migration: %w", err)
		}
		end, err := unix.Seek(int(in.Fd()), start, unix.SEEK_HOLE)
		if err != nil {
			return err
		}
		end = min(end, before.Size())
		if end <= start {
			return fmt.Errorf("invalid sparse extent")
		}
		if _, err = out.Seek(start, io.SeekStart); err != nil {
			return err
		}
		hash := sha256.New()
		reader := io.NewSectionReader(in, start, end-start)
		buf := make([]byte, 1024*1024)
		for { // Observe cancellation between reads.
			if err = ctx.Err(); err != nil {
				return err
			}
			n, e := reader.Read(buf)
			if n > 0 {
				hash.Write(buf[:n])
				if _, err = out.Write(buf[:n]); err != nil {
					return err
				}
			}
			if e == io.EOF {
				break
			}
			if e != nil {
				return e
			}
		}
		verify := sha256.New()
		if _, err = io.Copy(verify, io.NewSectionReader(out, start, end-start)); err != nil {
			return err
		}
		if !bytes.Equal(hash.Sum(nil), verify.Sum(nil)) {
			return fmt.Errorf("migration verification failed")
		}
		pos = end
	}
	after, err := in.Stat()
	if err != nil {
		return err
	}
	if after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return fmt.Errorf("source changed during migration")
	}
	return out.Sync()
}
