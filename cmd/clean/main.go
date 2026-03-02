// Standalone clean binary — polls PPID and removes the workspace directory
// once the parent process exits. Built with CGO_ENABLED=0 for static linking.
package main

import (
	"os"
	"time"

	"github.com/gofrs/flock"
	"github.com/sirupsen/logrus"
)

func main() {
	workspace := os.Getenv("WORKSPACE")
	payloadDir := os.Getenv("PAYLOAD_DIR")

	if workspace == "" && payloadDir == "" {
		return
	}

	f, err := os.OpenFile("/tmp/clean.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}

	logrus.SetOutput(f)

	for {
		if os.Getppid() == 1 {
			safeRemoveWorkspace(workspace)
			removeDir(payloadDir)
			return
		}
		time.Sleep(1 * time.Second)
	}
}

// safeRemoveWorkspace acquires an exclusive flock on <workspace>.lock
// (non-blocking) before deleting. If a new session with the same name
// is already running it holds that lock, so we skip deletion.
//
// The lock file is intentionally NOT removed — all sessions must flock
// the same inode; deleting and recreating produces a new inode that
// provides no mutual exclusion with the old one.
func safeRemoveWorkspace(workspace string) {
	if workspace == "" {
		return
	}

	lockPath := workspace + ".lock"
	fileLock := flock.New(lockPath)

	// Non-blocking exclusive lock.
	locked, err := fileLock.TryLock()
	if err != nil {
		removeDir(workspace)
		return
	}

	if !locked {
		return
	}
	removeDir(workspace)
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
