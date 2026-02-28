package docker

import (
	"log/slog"
	"os"
)

func init() {
	// Default: key=value to stdout for container-friendly parsing
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
}

// LogInfo logs an info message with key-value pairs.
func LogInfo(msg string, args ...any) { slog.Info(msg, args...) }

// LogError logs an error with key-value pairs.
func LogError(msg string, args ...any) { slog.Error(msg, args...) }

// LogWarn logs a warning with key-value pairs.
func LogWarn(msg string, args ...any) { slog.Warn(msg, args...) }
