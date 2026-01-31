package logger

import (
	"fmt"
	"os"

	"github.com/rs/zerolog"
)

type ZerologLogger struct {
	logger zerolog.Logger
}

type ZerologOptions struct {
	UseColor   bool
	Level      string
	TimeFormat string
	OutputFile string
}

func NewZerologLogger() Logger {
	return NewZerologLoggerWithOptions(ZerologOptions{
		UseColor:   true,
		Level:      "debug",
		TimeFormat: "15:04:05",
	})
}

func NewZerologLoggerWithOptions(opts ZerologOptions) Logger {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	if opts.TimeFormat != "" {
		zerolog.TimeFieldFormat = opts.TimeFormat
	}

	var logger zerolog.Logger

	if opts.OutputFile != "" {
		file, err := os.OpenFile(opts.OutputFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			panic(fmt.Sprintf("failed to open log file: %v", err))
		}
		logger = zerolog.New(file).With().Timestamp().Logger()
	} else if opts.UseColor {
		output := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: opts.TimeFormat,
		}
		logger = zerolog.New(output).With().Timestamp().Logger()
	} else {
		logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}

	switch opts.Level {
	case "debug":
		logger = logger.Level(zerolog.DebugLevel)
	case "info":
		logger = logger.Level(zerolog.InfoLevel)
	case "warn":
		logger = logger.Level(zerolog.WarnLevel)
	case "error":
		logger = logger.Level(zerolog.ErrorLevel)
	default:
		logger = logger.Level(zerolog.DebugLevel)
	}

	return &ZerologLogger{
		logger: logger,
	}
}

func (l *ZerologLogger) Debug(msg string, args ...any) {
	l.logger.Debug().Msg(fmt.Sprintf(msg, args...))
}

func (l *ZerologLogger) Info(msg string, args ...any) {
	l.logger.Info().Msg(fmt.Sprintf(msg, args...))
}

func (l *ZerologLogger) Warn(msg string, args ...any) {
	l.logger.Warn().Msg(fmt.Sprintf(msg, args...))
}

func (l *ZerologLogger) Error(msg string, args ...any) {
	l.logger.Error().Msg(fmt.Sprintf(msg, args...))
}
