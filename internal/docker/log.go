package docker

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
)

func init() {
	InitLogging()
}

// InitLogging reads LOG_LEVEL and LOG_FORMAT from the environment and sets the
// default slog handler. Call at process startup before any log output.
// LOG_LEVEL: DEBUG, INFO, WARN, ERROR (default INFO). Case-insensitive.
// LOG_FORMAT: compact, timestamp, json (default timestamp). Case-insensitive.
func InitLogging() {
	level := levelFromEnv()
	format := strings.TrimSpace(strings.ToLower(os.Getenv("LOG_FORMAT")))
	switch format {
	case "compact":
		slog.SetDefault(slog.New(newCompactHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
	case "json":
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
	default:
		slog.SetDefault(slog.New(newTimestampHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
	}
}

func levelFromEnv() slog.Level {
	switch strings.TrimSpace(strings.ToUpper(os.Getenv("LOG_LEVEL"))) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// compactHandlerStruct writes [LEVEL] message key=value...
type compactHandlerStruct struct {
	opts   *slog.HandlerOptions
	w      io.Writer
	attrs  []slog.Attr
	groups []string
}

func newCompactHandler(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &compactHandlerStruct{opts: opts, w: w}
}

func (h *compactHandlerStruct) Enabled(_ context.Context, level slog.Level) bool {
	min := slog.LevelInfo
	if h.opts != nil && h.opts.Level != nil {
		min = h.opts.Level.Level()
	}
	return level >= min
}

func appendAttr(buf []byte, prefix string, a slog.Attr) []byte {
	key := a.Key
	if prefix != "" {
		key = prefix + key
	}
	buf = append(buf, ' ')
	buf = append(buf, key...)
	buf = append(buf, '=')
	buf = append(buf, a.Value.String()...)
	return buf
}

func (h *compactHandlerStruct) Handle(_ context.Context, r slog.Record) error {
	buf := make([]byte, 0, 256)
	buf = append(buf, '[')
	buf = append(buf, r.Level.String()...)
	buf = append(buf, "] "...)
	buf = append(buf, r.Message...)
	prefix := strings.Join(h.groups, ".")
	if prefix != "" {
		prefix += "."
	}
	for _, a := range h.attrs {
		buf = appendAttr(buf, prefix, a)
	}
	r.Attrs(func(a slog.Attr) bool {
		buf = appendAttr(buf, prefix, a)
		return true
	})
	buf = append(buf, '\n')
	_, err := h.w.Write(buf)
	return err
}

func (h *compactHandlerStruct) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	clone := *h
	clone.attrs = make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	clone.attrs = append(clone.attrs, h.attrs...)
	clone.attrs = append(clone.attrs, attrs...)
	return &clone
}

func (h *compactHandlerStruct) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := *h
	clone.groups = make([]string, 0, len(h.groups)+1)
	clone.groups = append(clone.groups, h.groups...)
	clone.groups = append(clone.groups, name)
	return &clone
}

// timestampHandler writes 2006-01-02T15:04:05Z07:00 [LEVEL] message key=value...
type timestampHandlerStruct struct {
	opts   *slog.HandlerOptions
	w      io.Writer
	attrs  []slog.Attr
	groups []string
}

func newTimestampHandler(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &timestampHandlerStruct{opts: opts, w: w}
}

func (h *timestampHandlerStruct) Enabled(_ context.Context, level slog.Level) bool {
	min := slog.LevelInfo
	if h.opts != nil && h.opts.Level != nil {
		min = h.opts.Level.Level()
	}
	return level >= min
}

func (h *timestampHandlerStruct) Handle(_ context.Context, r slog.Record) error {
	buf := make([]byte, 0, 320)
	buf = r.Time.AppendFormat(buf, time.RFC3339)
	buf = append(buf, " ["...)
	buf = append(buf, r.Level.String()...)
	buf = append(buf, "] "...)
	buf = append(buf, r.Message...)
	prefix := strings.Join(h.groups, ".")
	if prefix != "" {
		prefix += "."
	}
	for _, a := range h.attrs {
		buf = appendAttr(buf, prefix, a)
	}
	r.Attrs(func(a slog.Attr) bool {
		buf = appendAttr(buf, prefix, a)
		return true
	})
	buf = append(buf, '\n')
	_, err := h.w.Write(buf)
	return err
}

func (h *timestampHandlerStruct) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	clone := *h
	clone.attrs = make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	clone.attrs = append(clone.attrs, h.attrs...)
	clone.attrs = append(clone.attrs, attrs...)
	return &clone
}

func (h *timestampHandlerStruct) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := *h
	clone.groups = make([]string, 0, len(h.groups)+1)
	clone.groups = append(clone.groups, h.groups...)
	clone.groups = append(clone.groups, name)
	return &clone
}

// LogDebug logs a debug message with key-value pairs.
func LogDebug(msg string, args ...any) { slog.Debug(msg, args...) }

// LogInfo logs an info message with key-value pairs.
func LogInfo(msg string, args ...any) { slog.Info(msg, args...) }

// LogWarn logs a warning with key-value pairs.
func LogWarn(msg string, args ...any) { slog.Warn(msg, args...) }

// LogError logs an error with key-value pairs.
func LogError(msg string, args ...any) { slog.Error(msg, args...) }
