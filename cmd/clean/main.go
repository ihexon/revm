// Standalone clean binary — polls PPID and removes the workspace directory
// once the parent process exits. Built with CGO_ENABLED=0 for static linking.
// Zero external dependencies — stdlib only.
package main

import (
	"log"
	"os"
	"time"
)

func main() {
	workspace := os.Getenv("WORKSPACE")
	payloadDir := os.Getenv("PAYLOAD_DIR")

	if workspace == "" && payloadDir == "" {
		return
	}

	for {
		if os.Getppid() == 1 {
			removeDir(workspace)
			removeDir(payloadDir)
			return
		}
		time.Sleep(1 * time.Second)
	}
}

func removeDir(dir string) {
	if dir == "" {
		return
	}
	log.Printf("PPID=1, removing %q", dir)
	if err := os.RemoveAll(dir); err != nil {
		log.Printf("failed to remove %q: %v", dir, err)
	}
}
