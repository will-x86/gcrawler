package logger

import (
	"fmt"
	"log/slog"
	"os"
)

type SlogLogger struct {
	logger *slog.Logger
}

func NewSlogLogger() Logger {
	return &SlogLogger{
		logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}
}

func (l *SlogLogger) Debug(msg string, args ...any) {
	l.logger.Debug(fmt.Sprintf(msg, args...))
}

func (l *SlogLogger) Info(msg string, args ...any) {
	l.logger.Info(fmt.Sprintf(msg, args...))
}

func (l *SlogLogger) Warn(msg string, args ...any) {
	l.logger.Warn(fmt.Sprintf(msg, args...))
}

func (l *SlogLogger) Error(msg string, args ...any) {
	l.logger.Error(fmt.Sprintf(msg, args...))
}
