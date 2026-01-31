package logger

import "log"

type StdLogger struct{}

func NewStdLogger() Logger {
	return &StdLogger{}
}

func (l *StdLogger) Debug(msg string, args ...any) {
	log.Printf("[DEBUG] "+msg, args...)
}

func (l *StdLogger) Info(msg string, args ...any) {
	log.Printf("[INFO] "+msg, args...)
}

func (l *StdLogger) Warn(msg string, args ...any) {
	log.Printf("[WARN] "+msg, args...)
}

func (l *StdLogger) Error(msg string, args ...any) {
	log.Printf("[ERROR] "+msg, args...)
}
