package docker

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
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
	mu     *sync.Mutex
	attrs  []slog.Attr
	groups []string
}

func newCompactHandler(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &compactHandlerStruct{opts: opts, w: w, mu: &sync.Mutex{}}
}

func (h *compactHandlerStruct) Enabled(_ context.Context, level slog.Level) bool {
	min := slog.LevelInfo
	if h.opts != nil && h.opts.Level != nil {
		min = h.opts.Level.Level()
	}
	return level >= min
}

// processAttr resolves a.Value (honoring LogValuer) and applies ReplaceAttr from
// HandlerOptions when non-nil, then resolves again. It does not apply ReplaceAttr
// to group attrs; the caller expands groups and processes sub-attrs separately.
func processAttr(groups []string, a slog.Attr, replaceAttr func([]string, slog.Attr) slog.Attr) slog.Attr {
	a.Value = a.Value.Resolve()
	if replaceAttr != nil && a.Value.Kind() != slog.KindGroup {
		a = replaceAttr(groups, a)
		a.Value = a.Value.Resolve()
	}
	return a
}

// appendProcessedAttrs appends each attr after resolving and applying replaceAttr,
// expands groups recursively, and skips attrs with empty key (discarded by ReplaceAttr).
func appendProcessedAttrs(buf []byte, prefix string, groups []string, attrs []slog.Attr, replaceAttr func([]string, slog.Attr) slog.Attr) []byte {
	for _, a := range attrs {
		a = processAttr(groups, a, replaceAttr)
		if a.Key == "" {
			continue
		}
		if a.Value.Kind() == slog.KindGroup {
			childPrefix := prefix
			if a.Key != "" {
				childPrefix = prefix + a.Key + "."
			}
			childGroups := make([]string, len(groups), len(groups)+1)
			copy(childGroups, groups)
			childGroups = append(childGroups, a.Key)
			buf = appendProcessedAttrs(buf, childPrefix, childGroups, a.Value.Group(), replaceAttr)
		} else {
			buf = appendAttr(buf, prefix, a)
		}
	}
	return buf
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
	var replaceAttr func([]string, slog.Attr) slog.Attr
	if h.opts != nil && h.opts.ReplaceAttr != nil {
		replaceAttr = h.opts.ReplaceAttr
	}
	buf = appendProcessedAttrs(buf, prefix, h.groups, h.attrs, replaceAttr)
	r.Attrs(func(a slog.Attr) bool {
		buf = appendProcessedAttrs(buf, prefix, h.groups, []slog.Attr{a}, replaceAttr)
		return true
	})
	buf = append(buf, '\n')
	h.mu.Lock()
	defer h.mu.Unlock()
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
	mu     *sync.Mutex
	attrs  []slog.Attr
	groups []string
}

func newTimestampHandler(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &timestampHandlerStruct{opts: opts, w: w, mu: &sync.Mutex{}}
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
	var replaceAttr func([]string, slog.Attr) slog.Attr
	if h.opts != nil && h.opts.ReplaceAttr != nil {
		replaceAttr = h.opts.ReplaceAttr
	}
	buf = appendProcessedAttrs(buf, prefix, h.groups, h.attrs, replaceAttr)
	r.Attrs(func(a slog.Attr) bool {
		buf = appendProcessedAttrs(buf, prefix, h.groups, []slog.Attr{a}, replaceAttr)
		return true
	})
	buf = append(buf, '\n')
	h.mu.Lock()
	defer h.mu.Unlock()
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
