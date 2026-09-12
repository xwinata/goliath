package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Level is the minimum severity a logger will emit. It maps onto slog levels.
type Level int

const (
	// LevelDebug logs everything.
	LevelDebug Level = iota
	// LevelInfo logs info, warn, and error. This is the zero value default.
	LevelInfo
	// LevelWarn logs warn and error.
	LevelWarn
	// LevelError logs only error.
	LevelError
)

func (l Level) slog() slog.Level {
	switch l {
	case LevelDebug:
		return slog.LevelDebug
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// fromSlog maps a slog.Level back to the nearest Level.
func fromSlog(l slog.Level) Level {
	switch {
	case l <= slog.LevelDebug:
		return LevelDebug
	case l < slog.LevelWarn:
		return LevelInfo
	case l < slog.LevelError:
		return LevelWarn
	default:
		return LevelError
	}
}

// ParseLevel resolves a string, int, or Level into a Level. Unknown values fall
// back to LevelInfo.
func ParseLevel(v any) Level {
	switch t := v.(type) {
	case Level:
		return t
	case int:
		l := Level(t)
		if l >= LevelDebug && l <= LevelError {
			return l
		}
		return LevelInfo
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "debug", "dbg":
			return LevelDebug
		case "warn", "wrn", "warning":
			return LevelWarn
		case "error", "err":
			return LevelError
		default:
			return LevelInfo
		}
	default:
		return LevelInfo
	}
}

// Writer selects an output destination.
type Writer int

const (
	// WriterStdout writes to stdout. Use WithFormat to choose JSON (default,
	// for containers) or text (human-readable, for development).
	WriterStdout Writer = iota
	// WriterFile writes to a daily-rotated file using the configured format.
	WriterFile
)

// Format selects how log records are encoded.
type Format int

const (
	// FormatJSON encodes each record as a JSON object (one per line). Default.
	FormatJSON Format = iota
	// FormatText encodes each record as human-readable key=value text.
	FormatText
)

// config is the resolved configuration produced by applying Options.
type config struct {
	level         Level
	format        Format
	writers       []Writer
	customWriters []io.Writer
	defaultFields map[string]any

	fileDir           string
	filePrefix        string
	fileBufKB         int
	fileFlushInterval int
}

func defaultConfig() config {
	prefix := "app"
	if host, _ := os.Hostname(); host != "" {
		prefix = "app-" + host
	}
	return config{
		level:             LevelInfo,
		format:            FormatJSON,
		writers:           []Writer{WriterStdout},
		fileDir:           "./logs/",
		filePrefix:        prefix,
		fileBufKB:         64,
		fileFlushInterval: 1,
	}
}

// Option customizes the logger via Init.
type Option func(*config)

// WithLevel sets the minimum log level.
func WithLevel(l Level) Option { return func(c *config) { c.level = l } }

// WithFormat sets the encoding used by all writers (JSON or text).
func WithFormat(f Format) Option { return func(c *config) { c.format = f } }

// WithWriters sets the built-in output destinations, replacing the default
// (WriterStdout). Pass no arguments to disable the built-in destinations
// entirely, e.g. when routing output solely through WithWriter.
func WithWriters(w ...Writer) Option {
	return func(c *config) {
		c.writers = w
	}
}

// WithWriter adds an arbitrary io.Writer as an output destination, using the
// configured format. It can be combined with WriterStdout and WriterFile and is
// useful for routing logs to a custom sink (or a buffer in tests).
func WithWriter(w io.Writer) Option {
	return func(c *config) {
		if w != nil {
			c.customWriters = append(c.customWriters, w)
		}
	}
}

// WithDefaultFields sets fields attached to every log entry.
func WithDefaultFields(fields map[string]any) Option {
	return func(c *config) { c.defaultFields = fields }
}

// WithFile configures the daily-rotated file writer. dir is the directory,
// prefix is the base file name (a date and .log suffix are appended).
func WithFile(dir, prefix string) Option {
	return func(c *config) {
		if dir != "" {
			c.fileDir = dir
		}
		if prefix != "" {
			c.filePrefix = prefix
		}
	}
}

// WithFileBuffer tunes the file writer's buffer size (KB) and flush interval
// (seconds). Non-positive values keep the defaults.
func WithFileBuffer(bufKB, flushSeconds int) Option {
	return func(c *config) {
		if bufKB > 0 {
			c.fileBufKB = bufKB
		}
		if flushSeconds > 0 {
			c.fileFlushInterval = flushSeconds
		}
	}
}

// buildState assembles the slog logger and resources from config.
//
// The inner handlers are created to accept every level (Level: LevelDebug); the
// wrapping levelHandler is the sole authority on filtering, so it can honor a
// context level that is lower than the global level.
func buildState(cfg config) (*state, error) {
	var (
		handlers []slog.Handler
		fw       *fileWriter
	)

	for _, w := range cfg.writers {
		switch w {
		case WriterStdout:
			handlers = append(handlers, handlerFor(cfg.format, os.Stdout))
		case WriterFile:
			writer, err := newFileWriter(
				filepath.Join(cfg.fileDir, cfg.filePrefix+".log"),
				cfg.fileBufKB,
				cfg.fileFlushInterval,
			)
			if err != nil {
				return nil, err
			}
			fw = writer
			handlers = append(handlers, handlerFor(cfg.format, writer))
		default:
			return nil, fmt.Errorf("logger: unsupported writer %d", w)
		}
	}

	for _, cw := range cfg.customWriters {
		handlers = append(handlers, handlerFor(cfg.format, cw))
	}

	if len(handlers) == 0 {
		return nil, fmt.Errorf("logger: no output writers configured")
	}

	var inner slog.Handler
	if len(handlers) == 1 {
		inner = handlers[0]
	} else {
		inner = newFanoutHandler(handlers)
	}

	// The level handler enforces the fixed global level and any per-context
	// override; the inner handlers accept every level.
	handler := newLevelHandler(cfg.level.slog(), inner)

	defaults := make([]slog.Attr, 0, len(cfg.defaultFields))
	for k, v := range cfg.defaultFields {
		defaults = append(defaults, slog.Any(k, v))
	}

	return &state{
		logger:        slog.New(handler),
		defaultFields: defaults,
		file:          fw,
	}, nil
}

// handlerFor builds a slog handler for the given format writing to w. Inner
// handlers accept every level; the wrapping levelHandler enforces filtering.
func handlerFor(format Format, w io.Writer) slog.Handler {
	opts := &slog.HandlerOptions{Level: slog.LevelDebug}
	if format == FormatText {
		return slog.NewTextHandler(w, opts)
	}
	return slog.NewJSONHandler(w, opts)
}
