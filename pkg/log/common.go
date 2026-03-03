package log

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/sirupsen/logrus"
)

const maxLogFileSize = 10 * 1024 * 1024

var logFileHolders struct {
	mu    sync.Mutex
	files []*os.File
}

func SetupBasicLogger(level string) error {
	l, err := logrus.ParseLevel(level)
	if err != nil {
		return fmt.Errorf("invalid log level: %w", err)
	}
	logrus.SetLevel(l)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05.000",
	})

	logrus.SetOutput(os.Stderr)
	return nil
}

func SetupBasicLoggerWithStage(level, stage string) error {
	if err := SetupBasicLogger(level); err != nil {
		return err
	}
	if stage != "" {
		logrus.AddHook(stageHook{stage: stage})
	}
	return nil
}

func SetupBasicLoggerWithStageAndFile(level, stage, logFilePath string) error {
	if err := SetupBasicLoggerWithStage(level, stage); err != nil {
		return err
	}
	if logFilePath == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(logFilePath), 0755); err != nil {
		return err
	}
	if info, err := os.Stat(logFilePath); err == nil && info.Size() > maxLogFileSize {
		_ = os.Truncate(logFilePath, 0)
	}

	f, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	logFileHolders.mu.Lock()
	logFileHolders.files = append(logFileHolders.files, f)
	logFileHolders.mu.Unlock()

	logrus.SetOutput(io.MultiWriter(os.Stderr, f))
	return nil
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
