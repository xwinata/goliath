package logger

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestContextLevel_RoundTrip(t *testing.T) {
	ctx := WithContextLevel(context.Background(), LevelWarn)

	lvl, ok := ContextLevel(ctx)
	if !ok {
		t.Fatal("expected a context level to be set")
	}
	if lvl != LevelWarn {
		t.Errorf("ContextLevel = %v, want %v", lvl, LevelWarn)
	}
}

func TestContextLevel_Absent(t *testing.T) {
	if lvl, ok := ContextLevel(context.Background()); ok {
		t.Errorf("expected no context level, got %v", lvl)
	}
}

func TestLevelFromContext_NilContext(t *testing.T) {
	//nolint:staticcheck // intentionally passing nil to verify the guard.
	if _, ok := levelFromContext(context.TODO()); ok {
		t.Error("expected no level from nil context")
	}
}

// recordingHandler captures records passed to Handle.
type recordingHandler struct {
	records []slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}
func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

func TestLevelHandler_EnabledUsesContextThenGlobal(t *testing.T) {
	h := newLevelHandler(slog.LevelInfo, &recordingHandler{})

	// No context level -> global (Info) applies.
	if h.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("debug should be disabled at global Info")
	}
	if !h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("info should be enabled at global Info")
	}

	// Context level overrides global (lowered to Debug).
	ctx := WithContextLevel(context.Background(), LevelDebug)
	if !h.Enabled(ctx, slog.LevelDebug) {
		t.Error("debug should be enabled when context level is Debug")
	}
}

func TestLevelHandler_HandleEnforcesLevel(t *testing.T) {
	rec := &recordingHandler{}
	h := newLevelHandler(slog.LevelWarn, rec)

	// Below the global level: Handle must drop it even without an Enabled check.
	below := slog.NewRecord(time.Now(), slog.LevelInfo, "below", 0)
	if err := h.Handle(context.Background(), below); err != nil {
		t.Fatal(err)
	}
	// At/above the level: forwarded.
	at := slog.NewRecord(time.Now(), slog.LevelError, "at", 0)
	if err := h.Handle(context.Background(), at); err != nil {
		t.Fatal(err)
	}

	if len(rec.records) != 1 {
		t.Fatalf("expected only 1 forwarded record, got %d", len(rec.records))
	}
	if rec.records[0].Message != "at" {
		t.Errorf("forwarded wrong record: %q", rec.records[0].Message)
	}
}
