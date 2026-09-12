package logger

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

// levelGate is a handler enabled only at or above a fixed level; it records how
// many records it handled.
type levelGate struct {
	min     slog.Level
	handled int
	err     error
}

func (g *levelGate) Enabled(_ context.Context, l slog.Level) bool { return l >= g.min }
func (g *levelGate) Handle(_ context.Context, _ slog.Record) error {
	g.handled++
	return g.err
}
func (g *levelGate) WithAttrs([]slog.Attr) slog.Handler { return g }
func (g *levelGate) WithGroup(string) slog.Handler      { return g }

func TestFanout_EnabledIfAnyEnabled(t *testing.T) {
	h := newFanoutHandler([]slog.Handler{
		&levelGate{min: slog.LevelError},
		&levelGate{min: slog.LevelDebug},
	})

	if !h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("fanout should be enabled if any sub-handler is enabled")
	}

	only := newFanoutHandler([]slog.Handler{&levelGate{min: slog.LevelError}})
	if only.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("fanout should be disabled when no sub-handler is enabled")
	}
}

func TestFanout_HandleOnlyEnabledSubs(t *testing.T) {
	errGate := &levelGate{min: slog.LevelError}
	dbgGate := &levelGate{min: slog.LevelDebug}
	h := newFanoutHandler([]slog.Handler{errGate, dbgGate})

	// An Info record: only the debug gate is enabled for it.
	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "hi", 0)
	if err := h.Handle(context.Background(), rec); err != nil {
		t.Fatal(err)
	}

	if errGate.handled != 0 {
		t.Errorf("error gate should not handle Info, got %d", errGate.handled)
	}
	if dbgGate.handled != 1 {
		t.Errorf("debug gate should handle Info once, got %d", dbgGate.handled)
	}
}

func TestFanout_JoinsErrors(t *testing.T) {
	boom := errors.New("boom")
	h := newFanoutHandler([]slog.Handler{
		&levelGate{min: slog.LevelDebug, err: boom},
		&levelGate{min: slog.LevelDebug},
	})

	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "hi", 0)
	err := h.Handle(context.Background(), rec)
	if !errors.Is(err, boom) {
		t.Errorf("expected joined error to contain boom, got %v", err)
	}
}
