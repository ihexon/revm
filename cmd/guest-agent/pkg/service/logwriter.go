package service

import (
	"io"
	"os"
	"sync"
)

var (
	stderrWriterMu sync.RWMutex
	stderrWriter   io.Writer = os.Stderr
)

func SetStderrWriter(w io.Writer) {
	stderrWriterMu.Lock()
	defer stderrWriterMu.Unlock()

	if w == nil {
		stderrWriter = os.Stderr
		return
	}
	stderrWriter = w
}

func StderrWriter() io.Writer {
	stderrWriterMu.RLock()
	defer stderrWriterMu.RUnlock()
	return stderrWriter
}
