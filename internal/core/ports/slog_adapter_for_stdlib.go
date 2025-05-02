package ports

import (
	"bytes"
	"log"
)

type stdLoggerAdapter struct {
	lg Logger
}

func (s *stdLoggerAdapter) Write(p []byte) (n int, err error) {
	msg := bytes.TrimSpace(p)
	s.lg.Error(string(msg))
	return len(p), nil
}

func NewSlogLogger(logger Logger) *log.Logger {
	return log.New(&stdLoggerAdapter{lg: logger}, "", 0)
}
