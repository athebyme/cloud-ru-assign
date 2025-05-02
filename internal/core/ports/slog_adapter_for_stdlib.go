package ports

import (
	"bytes"
	"log"
)

// stdLoggerAdapter оборачивает ports.Logger для совместимости с http.Server.ErrorLog
type stdLoggerAdapter struct {
	lg Logger
}

func (s *stdLoggerAdapter) Write(p []byte) (n int, err error) {
	// обрезаю лишние переносы строк, которые любит добавлять http.Server
	msg := bytes.TrimSpace(p)
	s.lg.Error(string(msg))
	return len(p), nil
}

// NewSlogLogger создает *log.Logger, который пишет в ports.Logger
// используется для передачи нашего логгера в http.Server
func NewSlogLogger(logger Logger) *log.Logger {
	// префикс и флаги стандартного логгера не нужны, тк этим занимается slog
	return log.New(&stdLoggerAdapter{lg: logger}, "", 0)
}
