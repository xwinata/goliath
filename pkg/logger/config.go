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
	// WriterStdout writes to stdout. Set Config.Format to choose JSON (default,
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

// Config customizes the logger via Init. The zero value is valid and yields the
// defaults: Info level, JSON format, output to stdout only, and the default
// daily-rotated file settings (used only when WriterFile is selected).
type Config struct {
	// Level is the minimum log level. The zero value is LevelInfo.
	Level Level

	// Format is the encoding used by all writers. The zero value is FormatJSON.
	Format Format

	// Writers selects the built-in output destinations. When nil, the default
	// (WriterStdout) is used. To emit to custom writers only, leave this nil
	// and set DisableBuiltinWriters.
	Writers []Writer

	// DisableBuiltinWriters drops the built-in destinations entirely, so output
	// goes only to CustomWriters. Use it when routing logs solely to a custom
	// sink (or a buffer in tests). It takes precedence over Writers.
	DisableBuiltinWriters bool

	// CustomWriters are arbitrary io.Writer destinations that receive output in
	// the configured format, alongside any built-in Writers. Nil entries are
	// ignored.
	CustomWriters []io.Writer

	// DefaultFields are attached to every log entry. The zero value adds none.
	DefaultFields map[string]any

	// FileDir is the directory for the daily-rotated file writer. Empty keeps
	// the default ("./logs/"). Used only when WriterFile is selected.
	FileDir string

	// FilePrefix is the base file name for the file writer (a date and .log
	// suffix are appended). Empty keeps the default ("app" or "app-<host>").
	FilePrefix string

	// FileBufferKB tunes the file writer's buffer size in KB. Values <= 0 keep
	// the default (64).
	FileBufferKB int

	// FileFlushSeconds tunes the file writer's flush interval in seconds.
	// Values <= 0 keep the default (1).
	FileFlushSeconds int
}

// resolved folds c into the internal config, applying defaults and the
// defensive validation the constructor relies on (ignore empty/nil, clamp
// non-positive file settings, drop nil custom writers).
func (c Config) resolved() config {
	cfg := defaultConfig()

	cfg.level = c.Level
	cfg.format = c.Format
	cfg.defaultFields = c.DefaultFields

	switch {
	case c.DisableBuiltinWriters:
		cfg.writers = nil
	case c.Writers != nil:
		cfg.writers = c.Writers
	}

	for _, w := range c.CustomWriters {
		if w != nil {
			cfg.customWriters = append(cfg.customWriters, w)
		}
	}

	if c.FileDir != "" {
		cfg.fileDir = c.FileDir
	}
	if c.FilePrefix != "" {
		cfg.filePrefix = c.FilePrefix
	}
	if c.FileBufferKB > 0 {
		cfg.fileBufKB = c.FileBufferKB
	}
	if c.FileFlushSeconds > 0 {
		cfg.fileFlushInterval = c.FileFlushSeconds
	}

	return cfg
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
