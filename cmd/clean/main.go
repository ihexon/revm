// Standalone clean binary — polls PPID and removes the workspace directory
// once the parent process exits. Built with CGO_ENABLED=0 for static linking.
package main

import (
	"log"
	"os"
	"time"

	"github.com/gofrs/flock"
)

func main() {
	workspace := os.Getenv("WORKSPACE")
	payloadDir := os.Getenv("PAYLOAD_DIR")

	if workspace == "" && payloadDir == "" {
		return
	}

	initialPPID := os.Getppid()

	for {
		if os.Getppid() != initialPPID {
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
		// Lock file gone or inaccessible — no active session, safe to remove leftovers.
		removeDir(workspace)
		return
	}

	if !locked {
		// A new session holds the lock, don't delete.
		log.Printf("session lock held, skipping cleanup of %q", workspace)
		return
	}
	removeDir(workspace)
	// Do NOT os.Remove(lockPath) — keep the inode stable for future flock coordination.
}

func removeDir(dir string) {
	if dir == "" {
		return
	}
	log.Printf("parent exited, removing %q", dir)
	if err := os.RemoveAll(dir); err != nil {
		log.Printf("failed to remove %q: %v", dir, err)
	}
}
