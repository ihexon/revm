//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/sirupsen/logrus"
)

const maxLogFileSize = 10 * 1024 * 1024

func (c *Config) WithLogging(level string, logFilePath string) *Config {
	if level == "" {
		level = logrus.InfoLevel.String()
	}

	c.LogLevel = level

	if logFilePath == "" {
		logFilePath = filepath.Join(getSessionDir(c.SessionID), "logs", "revm.log")
	}
	c.LogTo = logFilePath

	if err := setupLogrus(level); err != nil {
		panic(fmt.Sprintf("setup logging: %v", err))
	}
	logFile, err := setupLogFile(*c)
	if err != nil {
		panic(fmt.Sprintf("setup logging: %v", err))
	}
	setCurrentLogFile(logFile)

	return c
}

func setupLogrus(level string) error {
	if level == "" {
		level = logrus.InfoLevel.String()
	}

	l, err := logrus.ParseLevel(level)
	if err != nil {
		return fmt.Errorf("parse log level %q: %w", level, err)
	}

	logrus.SetLevel(l)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05.000",
		ForceColors:     true,
	})
	return nil
}

func setupLogFile(cfg Config) (*os.File, error) {
	logFilePath := cfg.LogTo
	if logFilePath == "" {
		logFilePath = filepath.Join(getSessionDir(cfg.SessionID), "logs", "revm.log")
	}

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

var currentRunLog struct {
	sync.Mutex
	file *os.File
}

func currentLogFile() *os.File {
	currentRunLog.Lock()
	defer currentRunLog.Unlock()
	return currentRunLog.file
}

func setCurrentLogFile(file *os.File) {
	currentRunLog.Lock()
	defer currentRunLog.Unlock()

	if currentRunLog.file != nil && currentRunLog.file != file {
		_ = currentRunLog.file.Close()
	}
	currentRunLog.file = file
	logrus.SetOutput(io.MultiWriter(os.Stderr, file))
}

func releaseLogFile(file *os.File) {
	currentRunLog.Lock()
	defer currentRunLog.Unlock()

	if file != nil && currentRunLog.file == file {
		logrus.SetOutput(os.Stderr)
		_ = file.Close()
		currentRunLog.file = nil
		return
	}
	if file != nil {
		_ = file.Close()
	}
}
