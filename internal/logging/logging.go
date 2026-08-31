package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
)

type Level = slog.Level

const (
	LevelDebug       = slog.LevelDebug
	LevelInfo        = slog.LevelInfo
	LevelWarn        = slog.LevelWarn
	LevelError       = slog.LevelError
	LevelFatal Level = 12
)

type Config struct {
	Level    Level
	FilePath string
}

type Service struct {
	logger *slog.Logger
	file   *os.File
}

func New(cfg Config) (*Service, error) {
	var writers []io.Writer
	writers = append(writers, os.Stdout)

	var file *os.File
	if cfg.FilePath != "" {
		f, err := os.OpenFile(cfg.FilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("logging: open file: %w", err)
		}
		file = f
		writers = append(writers, f)
	}

	handler := slog.NewJSONHandler(io.MultiWriter(writers...), &slog.HandlerOptions{
		Level: cfg.Level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey {
				if level, ok := a.Value.Any().(slog.Level); ok && level == LevelFatal {
					a.Value = slog.StringValue("FATAL")
				}
			}
			return a
		},
	})

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return &Service{logger: logger, file: file}, nil
}

func ParseLevel(s string) Level {
	switch s {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn":
		return LevelWarn
	case "error":
		return LevelError
	case "fatal":
		return LevelFatal
	default:
		return LevelInfo
	}
}

func (s *Service) Debug(msg string, args ...any) {
	s.logger.Debug(msg, args...)
}

func (s *Service) Info(msg string, args ...any) {
	s.logger.Info(msg, args...)
}

func (s *Service) Warn(msg string, args ...any) {
	s.logger.Warn(msg, args...)
}

func (s *Service) Error(msg string, args ...any) {
	s.logger.Error(msg, args...)
}

func (s *Service) Fatal(msg string, args ...any) {
	s.logger.Log(context.Background(), LevelFatal, msg, args...)
	s.Close()
	os.Exit(1)
}

// Logger returns the underlying slog logger.
func (s *Service) Logger() *slog.Logger {
	return s.logger
}

func (s *Service) Close() {
	if s.file != nil {
		s.file.Close()
	}
}
