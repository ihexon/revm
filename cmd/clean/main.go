// Standalone clean binary — polls PPID and removes the specified directories
// once the parent process exits. Built with CGO_ENABLED=0 for static linking.
//
// Usage: clean [workspace] [payload-dir] ...
package main

import (
	"os"
	"time"

	"github.com/gofrs/flock"
	"github.com/sirupsen/logrus"
)

func main() {
	dirs := os.Args[1:]
	if len(dirs) == 0 {
		return
	}

	f, err := os.OpenFile("/tmp/clean.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}

	logrus.SetOutput(f)

	for {
		if os.Getppid() == 1 {
			for _, dir := range dirs {
				safeRemoveDir(dir)
			}
			return
		}
		time.Sleep(1 * time.Second)
	}
}

func safeRemoveDir(dir string) {
	if dir == "" {
		return
	}

	lockPath := dir + ".lock"
	if _, err := os.Stat(lockPath); err != nil {
		// No lock file — not a workspace, just remove directly.
		removeDir(dir)
		return
	}

	fileLock := flock.New(lockPath)

	locked, err := fileLock.TryLock()
	if err != nil {
		logrus.Errorf("try lock %q failed: %v, skipping cleanup", lockPath, err)
		return
	}

	if !locked {
		logrus.Infof("lock held, skipping cleanup of %q", dir)
		return
	}
	removeDir(dir)
}

func removeDir(dir string) {
	if dir == "" {
		return
	}

	logrus.Infof("clean directory: %q", dir)
	if err := os.RemoveAll(dir); err != nil {
		logrus.Errorf("failed to remove directory: %v", err)
	}
}
