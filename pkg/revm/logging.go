//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

const maxLogFileSize = 10 * 1024 * 1024

// StartCommandLogging configures logrus and opens the run log for CLI preflight work.
func StartCommandLogging(cfg Config) (*os.File, error) {
	return setupRunLogging(cfg)
}

// StopCommandLogging releases a log file opened by StartCommandLogging.
func StopCommandLogging(file *os.File) {
	releaseRunLog(file)
}

func (c *Config) WithLogging(level string, logFilePath string) *Config {
	if level == "" {
		level = logrus.InfoLevel.String()
	}

	setupLogrus(level)
	c.LogLevel = level

	if logFilePath != "" {
		c.LogTo = logFilePath
	}

	return c
}

func setupRunLogging(cfg Config) (*os.File, error) {
	setupLogrus(cfg.LogLevel)
	logFile, err := setupLogFile(cfg)
	if err != nil {
		return nil, err
	}
	logrus.SetOutput(io.MultiWriter(os.Stderr, logFile))
	return logFile, nil
}

func setupLogrus(level string) {
	if level == "" {
		level = logrus.InfoLevel.String()
	}

	l, err := logrus.ParseLevel(level)
	if err != nil {
		panic(fmt.Errorf("parse log level %q: %w", level, err))
	}

	logrus.SetLevel(l)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05.000",
		ForceColors:     true,
	})
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

func releaseRunLog(file *os.File) {
	if file != nil {
		logrus.SetOutput(os.Stderr)
		_ = file.Close()
	}
}
