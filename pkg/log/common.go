package log

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

const maxLogFileSize = 10 * 1024 * 1024

// SetupLogger is the single logger setter for the project.
// stage and logFilePath are optional.
func SetupLogger(level, stage, logFilePath string) (*os.File, error) {
	l, err := logrus.ParseLevel(level)
	if err != nil {
		return nil, fmt.Errorf("invalid log level: %w", err)
	}
	logrus.SetLevel(l)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05.000",
		ForceColors:     true,
	})
	if stage != "" {
		logrus.AddHook(stageHook{stage: stage})
	}
	if logFilePath == "" {
		logrus.SetOutput(os.Stderr)
		return nil, nil
	}

	if err := os.MkdirAll(filepath.Dir(logFilePath), 0755); err != nil {
		return nil, err
	}
	if info, err := os.Stat(logFilePath); err == nil && info.Size() > maxLogFileSize {
		_ = os.Truncate(logFilePath, 0)
	}

	f, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	logrus.SetOutput(io.MultiWriter(os.Stderr, f))
	return f, nil
}

type stageHook struct {
	stage string
}

func (h stageHook) Levels() []logrus.Level { return logrus.AllLevels }

func (h stageHook) Fire(e *logrus.Entry) error {
	if h.stage == "" {
		return nil
	}
	if _, ok := e.Data["stage"]; !ok {
		e.Data["stage"] = h.stage
	}
	return nil
}
