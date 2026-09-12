package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
)

// parseLines decodes buffered JSON log output into a slice of records.
func parseLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("failed to parse log line %q: %v", line, err)
		}
		out = append(out, rec)
	}
	return out
}

// initBuf initializes the logger to write JSON into a fresh buffer at the given
// level, disabling the built-in stdout/file writers.
func initBuf(t *testing.T, level Level, opts ...Option) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	all := append([]Option{
		WithLevel(level),
		WithWriters(), // disable stdout/file
		WithWriter(buf),
		WithFormat(FormatJSON),
	}, opts...)
	if err := Init(all...); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return buf
}

func messages(recs []map[string]any) []string {
	msgs := make([]string, len(recs))
	for i, r := range recs {
		msgs[i], _ = r["msg"].(string)
	}
	return msgs
}

// ---------------------------------------------------------------------------
// Global level filtering
// ---------------------------------------------------------------------------

func TestMethods_RespectGlobalLevel(t *testing.T) {
	cases := []struct {
		name  string
		level Level
		// expected messages, in order, that should survive the level filter.
		want []string
	}{
		{"debug shows all", LevelDebug, []string{"dbg", "inf", "wrn", "err"}},
		{"info drops debug", LevelInfo, []string{"inf", "wrn", "err"}},
		{"warn drops debug+info", LevelWarn, []string{"wrn", "err"}},
		{"error drops all but error", LevelError, []string{"err"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := initBuf(t, tc.level)

			Debug("dbg")
			Info("inf")
			Warn("wrn")
			Error("err", nil)

			got := messages(parseLines(t, buf))
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("got messages %v, want %v", got, tc.want)
			}
		})
	}
}

func TestContextMethods_RespectGlobalLevel(t *testing.T) {
	buf := initBuf(t, LevelWarn)
	ctx := context.Background()

	DebugContext(ctx, "dbg")
	InfoContext(ctx, "inf")
	WarnContext(ctx, "wrn")
	ErrorContext(ctx, "err", nil)

	got := messages(parseLines(t, buf))
	want := []string{"wrn", "err"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}

// ---------------------------------------------------------------------------
// Context level applies to that call only (runtime override)
// ---------------------------------------------------------------------------

func TestContextLevel_AppliesToSingleCallOnly(t *testing.T) {
	buf := initBuf(t, LevelInfo)

	// A context lowered to Debug lets a debug line through...
	dbgCtx := WithContextLevel(context.Background(), LevelDebug)
	DebugContext(dbgCtx, "ctx-debug-shown")

	// ...but a debug call without that context is still filtered by the global
	// Info level.
	DebugContext(context.Background(), "plain-debug-dropped")
	Debug("bare-debug-dropped")

	got := messages(parseLines(t, buf))
	want := []string{"ctx-debug-shown"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestContextLevel_LowersThresholdForSingleCall(t *testing.T) {
	// Start strict: only errors pass under the global level.
	buf := initBuf(t, LevelError)

	// A context lowered to Debug lets a debug line through for that call only.
	dbgCtx := WithContextLevel(context.Background(), LevelDebug)
	DebugContext(dbgCtx, "ctx-debug-shown")

	// Default calls without that context still obey the global Error level, so
	// their debug/info/warn lines are dropped and only the error survives.
	Debug("bare-debug-dropped")
	Info("bare-info-dropped")
	Warn("bare-warn-dropped")
	Error("bare-error-shown", nil)

	got := messages(parseLines(t, buf))
	want := []string{"ctx-debug-shown", "bare-error-shown"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}

// ---------------------------------------------------------------------------
// Default fields always present
// ---------------------------------------------------------------------------

func TestDefaultFields_AlwaysPresent(t *testing.T) {
	buf := initBuf(t, LevelDebug, WithDefaultFields(map[string]any{
		"app_name": "goliath",
		"app_env":  "test",
	}))

	Debug("d", F("k", "v"))
	Info("i")
	Warn("w")
	Error("e", errors.New("boom"), F("op", "save"))

	recs := parseLines(t, buf)
	if len(recs) != 4 {
		t.Fatalf("expected 4 records, got %d", len(recs))
	}
	for _, r := range recs {
		if r["app_name"] != "goliath" {
			t.Errorf("record %q missing app_name: %v", r["msg"], r)
		}
		if r["app_env"] != "test" {
			t.Errorf("record %q missing app_env: %v", r["msg"], r)
		}
	}

	// The call-specific fields and the error are also present.
	if recs[0]["k"] != "v" {
		t.Errorf("expected field k=v on first record, got %v", recs[0]["k"])
	}
	if recs[3]["op"] != "save" {
		t.Errorf("expected field op=save on error record, got %v", recs[3]["op"])
	}
	if recs[3]["error"] != "boom" {
		t.Errorf("expected error=boom on error record, got %v", recs[3]["error"])
	}
}

func TestError_NilErrorOmitsErrorKey(t *testing.T) {
	buf := initBuf(t, LevelDebug)
	Error("no-err", nil)

	recs := parseLines(t, buf)
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	if _, ok := recs[0]["error"]; ok {
		t.Errorf("expected no error key when err is nil, got %v", recs[0]["error"])
	}
}

// ---------------------------------------------------------------------------
// Multiple writers
// ---------------------------------------------------------------------------

func TestMultipleWriters_ReceiveSameRecord(t *testing.T) {
	a := &bytes.Buffer{}
	b := &bytes.Buffer{}
	if err := Init(
		WithLevel(LevelInfo),
		WithWriters(), // disable stdout/file
		WithWriter(a),
		WithWriter(b),
		WithFormat(FormatJSON),
	); err != nil {
		t.Fatalf("Init: %v", err)
	}

	Info("hello", F("n", 1))
	Debug("dropped") // below level, should reach neither

	ra := parseLines(t, a)
	rb := parseLines(t, b)

	if len(ra) != 1 || len(rb) != 1 {
		t.Fatalf("expected 1 record per writer, got a=%d b=%d", len(ra), len(rb))
	}
	if ra[0]["msg"] != "hello" || rb[0]["msg"] != "hello" {
		t.Errorf("both writers should receive the record: a=%v b=%v", ra[0], rb[0])
	}
	// Level filtering is consistent across writers.
	if strings.Contains(a.String(), "dropped") || strings.Contains(b.String(), "dropped") {
		t.Errorf("below-level record leaked to a writer")
	}
}

// ---------------------------------------------------------------------------
// Concurrency
//
// The logger is package-global and expected to be called from many goroutines.
// These tests are most valuable under `go test -race`.
// ---------------------------------------------------------------------------

// lockedBuffer is a concurrency-safe io.Writer wrapper. A bare bytes.Buffer is
// not safe for concurrent writes, so the test must synchronize the sink itself
// to ensure it is exercising the logger rather than a buffer race.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestConcurrent_NoLostOrGarbledRecords(t *testing.T) {
	sink := &lockedBuffer{}
	if err := Init(
		WithLevel(LevelDebug),
		WithWriters(), // disable stdout/file
		WithWriter(sink),
		WithFormat(FormatJSON),
		WithDefaultFields(map[string]any{"app": "goliath"}),
	); err != nil {
		t.Fatalf("Init: %v", err)
	}

	const (
		goroutines = 50
		perG       = 20
	)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				// Mix of levels and a context override to exercise the level
				// handler concurrently. All levels are >= global Debug so every
				// call is expected to be emitted.
				switch i % 4 {
				case 0:
					Info("msg", F("g", g), F("i", i))
				case 1:
					Warn("msg", F("g", g), F("i", i))
				case 2:
					Error("msg", errors.New("e"), F("g", g), F("i", i))
				case 3:
					ctx := WithContextLevel(context.Background(), LevelDebug)
					DebugContext(ctx, "msg", F("g", g), F("i", i))
				}
			}
		}(g)
	}
	wg.Wait()

	// Every line must be complete, well-formed JSON with the default field and
	// the expected keys — no interleaving or truncation.
	lines := strings.Split(strings.TrimSpace(sink.String()), "\n")
	if len(lines) != goroutines*perG {
		t.Fatalf("expected %d records, got %d", goroutines*perG, len(lines))
	}
	for _, line := range lines {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("garbled log line %q: %v", line, err)
		}
		if rec["app"] != "goliath" {
			t.Errorf("record missing default field: %v", rec)
		}
		if _, ok := rec["g"]; !ok {
			t.Errorf("record missing field g: %v", rec)
		}
	}
}

func TestConcurrent_ContextLevelsAreIsolated(t *testing.T) {
	// Global is Error. Half the goroutines use a Debug context (their debug
	// lines should appear); the other half log debug with no context (dropped).
	sink := &lockedBuffer{}
	if err := Init(
		WithLevel(LevelError),
		WithWriters(),
		WithWriter(sink),
		WithFormat(FormatJSON),
	); err != nil {
		t.Fatalf("Init: %v", err)
	}

	const each = 100
	var wg sync.WaitGroup
	wg.Add(2)

	// Debug-context goroutine: every debug line should be emitted.
	go func() {
		defer wg.Done()
		ctx := WithContextLevel(context.Background(), LevelDebug)
		for i := 0; i < each; i++ {
			DebugContext(ctx, "shown", F("i", i))
		}
	}()

	// No-context goroutine: debug lines should all be dropped by global Error.
	go func() {
		defer wg.Done()
		for i := 0; i < each; i++ {
			Debug("dropped", F("i", i))
		}
	}()

	wg.Wait()

	lines := strings.Split(strings.TrimSpace(sink.String()), "\n")
	if len(lines) != each {
		t.Fatalf("expected %d emitted records, got %d", each, len(lines))
	}
	for _, line := range lines {
		if strings.Contains(line, "dropped") {
			t.Fatalf("a no-context debug line leaked through: %q", line)
		}
	}
}
