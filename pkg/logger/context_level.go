package logger

import (
	"context"
	"log/slog"
)

// levelKeyType is the unexported type for the context level key. Using a
// dedicated empty-struct type guarantees the key cannot collide with keys from
// other packages.
type levelKeyType struct{}

var levelKey levelKeyType

// WithContextLevel returns a copy of ctx carrying a log level that overrides the
// global level for logs made with that context. Use it to raise or lower
// verbosity for a single request or scope without affecting the rest of the
// process.
func WithContextLevel(ctx context.Context, level Level) context.Context {
	return context.WithValue(ctx, levelKey, level.slog())
}

// ContextLevel returns the level attached to ctx and whether one was set.
func ContextLevel(ctx context.Context) (Level, bool) {
	sl, ok := levelFromContext(ctx)
	if !ok {
		return LevelInfo, false
	}
	return fromSlog(sl), true
}

func levelFromContext(ctx context.Context) (slog.Level, bool) {
	if ctx == nil {
		return 0, false
	}
	sl, ok := ctx.Value(levelKey).(slog.Level)
	return sl, ok
}

// levelHandler enforces the effective level: it consults the context first and
// falls back to a fixed global level. The wrapped handler must accept all
// levels (it should be created with Level: LevelDebug) so this handler is the
// sole authority on what gets emitted.
type levelHandler struct {
	global slog.Level
	next   slog.Handler
}

func newLevelHandler(global slog.Level, next slog.Handler) slog.Handler {
	return &levelHandler{global: global, next: next}
}

func (h *levelHandler) effectiveLevel(ctx context.Context) slog.Level {
	if lv, ok := levelFromContext(ctx); ok {
		return lv
	}
	return h.global
}

func (h *levelHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.effectiveLevel(ctx)
}

func (h *levelHandler) Handle(ctx context.Context, r slog.Record) error {
	// Re-check the level here so filtering is consistent regardless of how the
	// handler is reached. slog.Logger checks Enabled first as an optimization,
	// but Handle must not rely on that: any caller that invokes Handle directly
	// (or a future writer) must still honor the effective level.
	if r.Level < h.effectiveLevel(ctx) {
		return nil
	}
	return h.next.Handle(ctx, r)
}

func (h *levelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &levelHandler{global: h.global, next: h.next.WithAttrs(attrs)}
}

func (h *levelHandler) WithGroup(name string) slog.Handler {
	return &levelHandler{global: h.global, next: h.next.WithGroup(name)}
}
