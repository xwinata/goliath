package logger

import (
	"context"
	"errors"
	"log/slog"
)

// fanoutHandler dispatches each record to multiple underlying slog handlers.
// slog does not ship a tee handler, so this provides one with no dependencies.
type fanoutHandler struct {
	handlers []slog.Handler
}

func newFanoutHandler(handlers []slog.Handler) slog.Handler {
	return &fanoutHandler{handlers: handlers}
}

// Enabled reports true if any underlying handler is enabled for the level.
func (h *fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, sub := range h.handlers {
		if sub.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle forwards the record to every handler that is enabled for its level,
// collecting any errors.
func (h *fanoutHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, sub := range h.handlers {
		if !sub.Enabled(ctx, r.Level) {
			continue
		}
		if err := sub.Handle(ctx, r.Clone()); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (h *fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	subs := make([]slog.Handler, len(h.handlers))
	for i, sub := range h.handlers {
		subs[i] = sub.WithAttrs(attrs)
	}
	return &fanoutHandler{handlers: subs}
}

func (h *fanoutHandler) WithGroup(name string) slog.Handler {
	subs := make([]slog.Handler, len(h.handlers))
	for i, sub := range h.handlers {
		subs[i] = sub.WithGroup(name)
	}
	return &fanoutHandler{handlers: subs}
}
