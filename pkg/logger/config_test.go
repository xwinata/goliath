package logger

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	cases := []struct {
		in   any
		want Level
	}{
		{"debug", LevelDebug},
		{"DBG", LevelDebug},
		{" Info ", LevelInfo},
		{"warn", LevelWarn},
		{"warning", LevelWarn},
		{"error", LevelError},
		{"err", LevelError},
		{"nonsense", LevelInfo},
		{LevelWarn, LevelWarn},
		{int(LevelError), LevelError},
		{int(99), LevelInfo}, // out of range
		{3.14, LevelInfo},    // unsupported type
	}
	for _, tc := range cases {
		if got := ParseLevel(tc.in); got != tc.want {
			t.Errorf("ParseLevel(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestLevel_SlogRoundTrip(t *testing.T) {
	cases := []struct {
		lvl  Level
		slog slog.Level
	}{
		{LevelDebug, slog.LevelDebug},
		{LevelInfo, slog.LevelInfo},
		{LevelWarn, slog.LevelWarn},
		{LevelError, slog.LevelError},
	}
	for _, tc := range cases {
		if got := tc.lvl.slog(); got != tc.slog {
			t.Errorf("%v.slog() = %v, want %v", tc.lvl, got, tc.slog)
		}
		if got := fromSlog(tc.slog); got != tc.lvl {
			t.Errorf("fromSlog(%v) = %v, want %v", tc.slog, got, tc.lvl)
		}
	}
}

func TestWithWriters_DisablesBuiltins(t *testing.T) {
	// No built-in writers and no custom writer -> build error.
	err := Init(WithWriters())
	if err == nil {
		t.Fatal("expected error when no writers are configured")
	}
	if !strings.Contains(err.Error(), "no output writers") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWithWriter_NilIgnored(t *testing.T) {
	// A nil custom writer should be ignored; with no built-ins this errors.
	if err := Init(WithWriters(), WithWriter(nil)); err == nil {
		t.Fatal("expected error: nil writer should not count as a destination")
	}
}

func TestWithFormat_Text(t *testing.T) {
	buf := &bytes.Buffer{}
	if err := Init(
		WithLevel(LevelInfo),
		WithWriters(),
		WithWriter(buf),
		WithFormat(FormatText),
	); err != nil {
		t.Fatalf("Init: %v", err)
	}

	Info("hello", F("user_id", 7))

	out := buf.String()
	// Text handler emits key=value, not JSON.
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("expected text output, got JSON-looking line: %q", out)
	}
	if !strings.Contains(out, "msg=hello") || !strings.Contains(out, "user_id=7") {
		t.Errorf("text output missing expected fields: %q", out)
	}
}
