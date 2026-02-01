package service

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

//go:embed busybox.static
var busyboxBytes []byte

//go:embed dropbearmulti
var dropbearmultiBytes []byte

// EmbeddedBinary represents an embedded binary that can be extracted to disk.
type EmbeddedBinary struct {
	name  string
	bytes []byte
	path  string
	once  sync.Once
}

var (
	BusyboxBinary       = &EmbeddedBinary{name: "busybox", bytes: busyboxBytes}
	DropbearmultiBinary = &EmbeddedBinary{name: "dropbearmulti", bytes: dropbearmultiBytes}
)

// Extract writes the embedded binary to the specified directory.
// It's safe to call multiple times - extraction only happens once.
func (e *EmbeddedBinary) Extract(dir string) (string, error) {
	var extractErr error

	e.once.Do(func() {
		if err := os.MkdirAll(dir, 0755); err != nil {
			extractErr = fmt.Errorf("create dir %s: %w", dir, err)
			return
		}

		e.path = filepath.Join(dir, e.name)

		// Skip if already exists
		if _, err := os.Stat(e.path); err == nil {
			return
		}

		if err := os.WriteFile(e.path, e.bytes, 0755); err != nil {
			extractErr = fmt.Errorf("write %s: %w", e.path, err)
			return
		}
	})

	if extractErr != nil {
		return "", extractErr
	}

	if e.path == "" {
		return "", fmt.Errorf("binary %s not extracted", e.name)
	}

	return e.path, nil
}

// Path returns the extracted binary path. Must call Extract first.
func (e *EmbeddedBinary) Path() string {
	return e.path
}
