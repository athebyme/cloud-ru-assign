package logadapter

import (
	"github.com/athebyme/cloud-ru-assign/internal/core/ports"
	"log/slog"
	"os"
)

// SlogAdapter реализует порт ports.Logger, используя стандартный пакет log/slog
type SlogAdapter struct {
	logger *slog.Logger
}

// NewSlogAdapter создает новый адаптер slog логгера
// levelStr задает минимальный уровень логирования (debug, info, warn, error)
// isJSON определяет формат вывода (JSON или Text)
func NewSlogAdapter(levelStr string, isJSON bool) *SlogAdapter {
	var level slog.Level
	switch levelStr {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo // по умолчанию
	}

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: true, // имя файла и строки откуда был вызов
	}

	var handler slog.Handler // выбираем обработчик (формат вывода)
	if isJSON {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	return &SlogAdapter{logger: logger}
}

// ------------ Реализации для ports.Logger -------------------

func (s *SlogAdapter) Debug(msg string, args ...any) {
	s.logger.Debug(msg, args...)
}

func (s *SlogAdapter) Info(msg string, args ...any) {
	s.logger.Info(msg, args...)
}

func (s *SlogAdapter) Warn(msg string, args ...any) {
	s.logger.Warn(msg, args...)
}

func (s *SlogAdapter) Error(msg string, args ...any) {
	s.logger.Error(msg, args...)
}

func (s *SlogAdapter) With(args ...any) ports.Logger {
	newLogger := s.logger.With(args...)
	return &SlogAdapter{logger: newLogger}
}
