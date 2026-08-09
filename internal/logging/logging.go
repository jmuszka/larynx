package logging

import (
	"context"
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

func New(cfg Config) *Service {
	var writers []io.Writer
	writers = append(writers, os.Stdout)

	if cfg.FilePath != "" {
		f, err := os.OpenFile(cfg.FilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			panic(err)
		}
		writers = append(writers, f)
	}

	handler := slog.NewJSONHandler(io.MultiWriter(writers...), &slog.HandlerOptions{
		Level: cfg.Level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey {
				level := a.Value.Any().(slog.Level)
				if level == LevelFatal {
					a.Value = slog.StringValue("FATAL")
				}
			}
			return a
		},
	})

	logger := slog.New(handler)
	slog.SetDefault(logger)

	svc := &Service{logger: logger}
	if cfg.FilePath != "" {
		svc.file = writers[1].(*os.File)
	}

	return svc
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

func (s *Service) Close() {
	if s.file != nil {
		s.file.Close()
	}
}
