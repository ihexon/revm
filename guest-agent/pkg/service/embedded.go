package service

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/sirupsen/logrus"
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

// ExtractToDir writes the embedded binary to the specified directory.
// It's safe to call multiple times - extraction only happens once.
func (e *EmbeddedBinary) ExtractToDir(dir string) (string, error) {
	var extractErr error

	e.once.Do(func() {
		if err := os.MkdirAll(dir, 0755); err != nil {
			extractErr = err
			return
		}

		e.path = filepath.Join(dir, e.name)

		// Skip if already exists
		if _, err := os.Stat(e.path); err == nil {
			logrus.Debugf("embedded binary %q already exists, skipping extraction", e.path)
			return
		}

		logrus.Debugf("extracting embedded binary %q to %q", e.name, e.path)
		if err := os.WriteFile(e.path, e.bytes, 0755); err != nil {
			extractErr = err
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

// Path returns the extracted binary path. Must call ExtractToDir first.
func (e *EmbeddedBinary) Path() string {
	return e.path
}
