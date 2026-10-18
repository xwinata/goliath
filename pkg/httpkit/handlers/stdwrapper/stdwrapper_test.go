package stdwrapper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"goliath/pkg/logger"
	"goliath/pkg/standarderror"
)

// captureLogs routes the package logger to a buffer for the duration of the
// test and returns the buffer. It restores the default logger on cleanup.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	if err := logger.Init(logger.Config{
		DisableBuiltinWriters: true,
		CustomWriters:         []io.Writer{buf},
		Level:                 logger.LevelDebug,
		Format:                logger.FormatJSON,
	}); err != nil {
		t.Fatalf("logger.Init: %v", err)
	}
	t.Cleanup(func() { _ = logger.Init(logger.Config{}) })
	return buf
}

// do serves req against the wrapped handler and returns the recorder.
func do(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) envelope {
	t.Helper()
	var env envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return env
}

func TestSuccessEnvelopeAndStatus(t *testing.T) {
	captureLogs(t)
	h := Wrap(func(http.ResponseWriter, *http.Request) (any, error) {
		return map[string]string{"name": "alice"}, nil
	}, Config{})

	rec := do(h, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	env := decodeEnvelope(t, rec)
	if env.Message != defaultSuccessMessage {
		t.Fatalf("message = %q, want %q", env.Message, defaultSuccessMessage)
	}
	data, ok := env.Data.(map[string]any)
	if !ok || data["name"] != "alice" {
		t.Fatalf("data = %#v, want name=alice", env.Data)
	}
}

func TestRequestIDGeneratedWhenAbsent(t *testing.T) {
	captureLogs(t)
	var seen string
	h := Wrap(func(_ http.ResponseWriter, r *http.Request) (any, error) {
		id, ok := RequestIDFromContext(r.Context())
		if !ok {
			t.Fatal("request ID missing from context")
		}
		seen = id
		return nil, nil
	}, Config{})

	rec := do(h, httptest.NewRequest(http.MethodGet, "/", nil))

	got := rec.Header().Get(HeaderRequestID)
	if got == "" {
		t.Fatal("response missing X-Request-ID header")
	}
	if got != seen {
		t.Fatalf("header id %q != context id %q", got, seen)
	}
}

func TestRequestIDPropagatedFromHeader(t *testing.T) {
	captureLogs(t)
	h := Wrap(func(http.ResponseWriter, *http.Request) (any, error) {
		return nil, nil
	}, Config{})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(HeaderRequestID, "abc-123")
	rec := do(h, req)

	if got := rec.Header().Get(HeaderRequestID); got != "abc-123" {
		t.Fatalf("X-Request-ID = %q, want %q", got, "abc-123")
	}
}

func TestErrorStatusAndErrorCodeHeader(t *testing.T) {
	captureLogs(t)
	h := Wrap(func(http.ResponseWriter, *http.Request) (any, error) {
		return nil, standarderror.New(standarderror.Params{
			Message: "user not found",
			Code:    "USER_NOT_FOUND",
			Status:  http.StatusNotFound,
		})
	}, Config{})

	rec := do(h, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if got := rec.Header().Get(HeaderErrorCode); got != "USER_NOT_FOUND" {
		t.Fatalf("X-Error-Code = %q, want %q", got, "USER_NOT_FOUND")
	}
	env := decodeEnvelope(t, rec)
	if env.Message != "user not found" {
		t.Fatalf("message = %q, want %q", env.Message, "user not found")
	}
	if env.Data != nil {
		t.Fatalf("data = %#v, want nil on error", env.Data)
	}
}

func TestPlainErrorDefaultsToInternalServerError(t *testing.T) {
	captureLogs(t)
	h := Wrap(func(http.ResponseWriter, *http.Request) (any, error) {
		return nil, errors.New("boom")
	}, Config{})

	rec := do(h, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if got := rec.Header().Get(HeaderErrorCode); got != "" {
		t.Fatalf("X-Error-Code = %q, want empty for plain error", got)
	}
	// A plain error's message must not leak to the client; the generic default
	// is used instead.
	env := decodeEnvelope(t, rec)
	if env.Message != defaultErrorMessage {
		t.Fatalf("message = %q, want generic default %q", env.Message, defaultErrorMessage)
	}
	if strings.Contains(env.Message, "boom") {
		t.Fatalf("message %q leaked the internal error text", env.Message)
	}
}

func TestErrorIsLogged(t *testing.T) {
	buf := captureLogs(t)
	h := Wrap(func(http.ResponseWriter, *http.Request) (any, error) {
		return nil, standarderror.New(standarderror.Params{
			Message: "kaboom",
			Code:    "X",
			Status:  http.StatusBadGateway,
		})
	}, Config{})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(HeaderRequestID, "rid-1")
	do(h, req)

	out := buf.String()
	if !strings.Contains(out, "request failed") {
		t.Fatalf("log missing failure message: %s", out)
	}
	if !strings.Contains(out, "rid-1") {
		t.Fatalf("log missing request id: %s", out)
	}
}

func TestPanicRecovered(t *testing.T) {
	buf := captureLogs(t)
	h := Wrap(func(http.ResponseWriter, *http.Request) (any, error) {
		panic("unexpected")
	}, Config{})

	rec := do(h, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	env := decodeEnvelope(t, rec)
	if env.Message != defaultErrorMessage {
		t.Fatalf("message = %q, want %q", env.Message, defaultErrorMessage)
	}
	if rec.Header().Get(HeaderRequestID) == "" {
		t.Fatal("panic response missing X-Request-ID header")
	}
	if !strings.Contains(buf.String(), "request panicked") {
		t.Fatalf("log missing panic message: %s", buf.String())
	}
	// The logged stack must reach the panic's origin (the wrapped handler),
	// not just the recovery site, and record the panic value.
	out := buf.String()
	if !strings.Contains(out, "TestPanicRecovered") {
		t.Fatalf("stack missing panic origin frame: %s", out)
	}
	if !strings.Contains(out, "unexpected") {
		t.Fatalf("log missing panic value: %s", out)
	}
}

func TestWithMessagesOverride(t *testing.T) {
	captureLogs(t)
	h := Wrap(
		func(http.ResponseWriter, *http.Request) (any, error) {
			return nil, nil
		},
		Config{SuccessMessage: "created"},
	)

	rec := do(h, httptest.NewRequest(http.MethodGet, "/", nil))
	env := decodeEnvelope(t, rec)
	if env.Message != "created" {
		t.Fatalf("message = %q, want %q", env.Message, "created")
	}
}

func TestContextCarriesRequestID(t *testing.T) {
	id, ok := RequestIDFromContext(context.Background())
	if ok || id != "" {
		t.Fatalf("empty context should have no request id, got %q, %v", id, ok)
	}
}

func TestCustomRequestIDHeader(t *testing.T) {
	captureLogs(t)
	h := Wrap(
		func(_ http.ResponseWriter, r *http.Request) (any, error) {
			if id, ok := RequestIDFromContext(r.Context()); !ok || id != "trace-9" {
				t.Fatalf("context request id = %q (%v), want %q", id, ok, "trace-9")
			}
			return nil, nil
		},
		Config{RequestIDHeader: "X-Trace-Id"},
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Trace-Id", "trace-9")
	rec := do(h, req)

	if got := rec.Header().Get("X-Trace-Id"); got != "trace-9" {
		t.Fatalf("X-Trace-Id = %q, want %q", got, "trace-9")
	}
	// The default header must not be used when overridden.
	if got := rec.Header().Get(HeaderRequestID); got != "" {
		t.Fatalf("default %s = %q, want empty when overridden", HeaderRequestID, got)
	}
}

func TestCustomErrorCodeHeader(t *testing.T) {
	captureLogs(t)
	h := Wrap(
		func(http.ResponseWriter, *http.Request) (any, error) {
			return nil, standarderror.New(standarderror.Params{
				Message: "nope",
				Code:    "E_NOPE",
				Status:  http.StatusBadRequest,
			})
		},
		Config{ErrorCodeHeader: "X-App-Error"},
	)

	rec := do(h, httptest.NewRequest(http.MethodGet, "/", nil))

	if got := rec.Header().Get("X-App-Error"); got != "E_NOPE" {
		t.Fatalf("X-App-Error = %q, want %q", got, "E_NOPE")
	}
	if got := rec.Header().Get(HeaderErrorCode); got != "" {
		t.Fatalf("default %s = %q, want empty when overridden", HeaderErrorCode, got)
	}
}
