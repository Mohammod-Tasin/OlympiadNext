package logger

import (
	"log/slog"
	"os"
)

// New builds a structured JSON logger. Text handler is used in
// development for readability; JSON in every other environment so logs
// are machine-parseable in production.
func New(env string) *slog.Logger {
	level := slog.LevelInfo
	if env == "development" {
		level = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if env == "development" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	logger := slog.New(handler).With("env", env)
	slog.SetDefault(logger)
	return logger
}
