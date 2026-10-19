//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

const maxLogFileSize = 10 * 1024 * 1024

// Bound automatic diagnostics even when a failing kernel is flooding its log.
func guestLogTail(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return ""
	}
	start := max(int64(0), info.Size()-2048)
	buf := make([]byte, info.Size()-start)
	n, _ := f.ReadAt(buf, start)
	lines := strings.Split(strings.TrimSpace(strings.ToValidUTF8(string(buf[:n]), "?")), "\n")
	if start > 0 && len(lines) > 1 {
		lines = lines[1:]
	}
	return strings.Join(lines[max(0, len(lines)-8):], "\n")
}

// StartCommandLogging configures logrus and opens the run log for CLI preflight work.
func StartCommandLogging(cfg Config) (*os.File, error) {
	return setupRunLogging(cfg)
}

// StopCommandLogging releases a log file opened by StartCommandLogging.
func StopCommandLogging(file *os.File) {
	releaseRunLog(file)
}

func setupRunLogging(cfg Config) (*os.File, error) {
	if err := validateSessionID(cfg.SessionID); err != nil {
		return nil, err
	}
	if err := setupLogrus(cfg.LogLevel); err != nil {
		return nil, err
	}
	logFile, err := setupLogFile(cfg)
	if err != nil {
		return nil, err
	}
	// Keep host lifecycle logs out of the command's stdout/stderr streams. In
	// TTY mode those streams are the guest console, so writing both streams at
	// once makes log lines interleave with command output and corrupt the
	// terminal layout. Callers can follow the session log when they need the
	// lifecycle trace.
	if logFile != nil {
		logrus.SetOutput(logFile)
	} else {
		logrus.SetOutput(io.Discard)
	}
	return logFile, nil
}

func setupLogrus(level string) error {
	if level == "" {
		level = logrus.InfoLevel.String()
	}

	l, err := logrus.ParseLevel(level)
	if err != nil {
		return fmt.Errorf("invalid --log-level %q: %w", level, err)
	}

	logrus.SetLevel(l)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05.000",
	})
	return nil
}

func setupLogFile(cfg Config) (*os.File, error) {
	if cfg.paths == nil {
		return nil, nil
	}
	logFilePath := filepath.Join(cfg.paths.logDir, "revm.log")

	if err := os.MkdirAll(filepath.Dir(logFilePath), 0755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	if info, err := os.Stat(logFilePath); err == nil && info.Size() > maxLogFileSize {
		if err := os.Truncate(logFilePath, 0); err != nil {
			return nil, fmt.Errorf("truncate log file: %w", err)
		}
	}

	f, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	return f, nil
}

func releaseRunLog(file *os.File) {
	if file != nil {
		// CLI diagnostics have a separate writer. Late network callbacks must
		// never escape into a workload's stderr after its log closes.
		logrus.SetOutput(io.Discard)
		_ = file.Close()
	}
}
