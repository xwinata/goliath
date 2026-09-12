// Package logger provides structured logging built entirely on the standard
// library's log/slog. It has no third-party dependencies.
//
// It supports multiple output destinations (stdout and a daily-rotated file)
// with a selectable format (JSON or text), a fixed level chosen at
// initialization, and default fields attached to every entry.
//
// # Initialization
//
//	logger.Init(
//	    logger.WithLevel(logger.LevelInfo),
//	    logger.WithWriters(logger.WriterStdout),
//	    logger.WithFormat(logger.FormatJSON),
//	    logger.WithDefaultFields(map[string]any{"app_name": "goliath"}),
//	)
//
// The global level is set once at Init and does not change at runtime. To raise
// or lower verbosity for a specific request or scope, attach a level to the
// context; it takes precedence over the global level for logs made with that
// context:
//
//	ctx = logger.WithContextLevel(ctx, logger.LevelDebug)
//	logger.DebugContext(ctx, "detailed trace", logger.F("step", 1))
//
// # Usage
//
//	logger.Info("request processed", logger.F("user_id", 123))
//	logger.Error("save failed", err, logger.F("op", "save"))
//
// Before Init is called, a default logger writing JSON to stdout at Info level
// is used, so logging never panics on an uninitialized package.
package logger

import (
	"context"
	"log/slog"
	"os"
	"sync"
)

// state holds the active logger and any resources that need cleanup.
type state struct {
	logger        *slog.Logger
	defaultFields []slog.Attr
	file          *fileWriter
}

var (
	mu      sync.RWMutex
	current = defaultState()
)

// defaultState returns a logger writing JSON to stdout at Info level. It is used
// until Init is called so that the package is safe to use out of the box.
func defaultState() *state {
	handler := newLevelHandler(
		slog.LevelInfo,
		slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}),
	)
	return &state{logger: slog.New(handler)}
}

// Init configures the package-level logger. Calling Init again replaces the
// previous configuration; any previously opened log file is closed first.
func Init(opts ...Option) error {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	newState, err := buildState(cfg)
	if err != nil {
		return err
	}

	mu.Lock()
	old := current
	current = newState
	mu.Unlock()

	// Release the previous file writer, if any, outside the lock.
	if old != nil && old.file != nil {
		_ = old.file.Close()
	}
	return nil
}

func active() *state {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Debug logs a message at debug level with optional structured fields.
func Debug(msg string, fields ...Field) {
	log(context.Background(), slog.LevelDebug, msg, nil, fields)
}

// Info logs a message at info level with optional structured fields.
func Info(msg string, fields ...Field) {
	log(context.Background(), slog.LevelInfo, msg, nil, fields)
}

// Warn logs a message at warn level with optional structured fields.
func Warn(msg string, fields ...Field) {
	log(context.Background(), slog.LevelWarn, msg, nil, fields)
}

// Error logs a message at error level. The err is attached under the "error"
// key when non-nil.
func Error(msg string, err error, fields ...Field) {
	log(context.Background(), slog.LevelError, msg, err, fields)
}

// DebugContext logs at debug level, honoring any context-attached level.
func DebugContext(ctx context.Context, msg string, fields ...Field) {
	log(ctx, slog.LevelDebug, msg, nil, fields)
}

// InfoContext logs at info level, honoring any context-attached level.
func InfoContext(ctx context.Context, msg string, fields ...Field) {
	log(ctx, slog.LevelInfo, msg, nil, fields)
}

// WarnContext logs at warn level, honoring any context-attached level.
func WarnContext(ctx context.Context, msg string, fields ...Field) {
	log(ctx, slog.LevelWarn, msg, nil, fields)
}

// ErrorContext logs at error level, honoring any context-attached level. The
// err is attached under the "error" key when non-nil.
func ErrorContext(ctx context.Context, msg string, err error, fields ...Field) {
	log(ctx, slog.LevelError, msg, err, fields)
}

// Sync flushes any buffered output (currently the file writer). It is safe to
// call even when logging to stdout only.
func Sync() error {
	st := active()
	if st.file != nil {
		return st.file.Flush()
	}
	return nil
}

func log(ctx context.Context, level slog.Level, msg string, err error, fields []Field) {
	st := active()
	if !st.logger.Enabled(ctx, level) {
		return
	}

	attrs := make([]slog.Attr, 0, len(st.defaultFields)+len(fields)+1)
	attrs = append(attrs, st.defaultFields...)
	for _, f := range fields {
		attrs = append(attrs, f.attr())
	}
	if err != nil {
		attrs = append(attrs, slog.Any("error", err))
	}

	st.logger.LogAttrs(ctx, level, msg, attrs...)
}
