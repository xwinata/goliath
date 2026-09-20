package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newServer builds a test server with the handler mounted on a fresh mux.
func newServer(t *testing.T, opts ...Option) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	New(opts...).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// get issues a GET and returns the status code and decoded body.
func get(t *testing.T, url string) (int, response) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()

	var body response
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return resp.StatusCode, body
}

func TestHealthAlwaysOK(t *testing.T) {
	srv := newServer(t)

	code, body := get(t, srv.URL+DefaultHealthPath)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d", code, http.StatusOK)
	}
	if body.Status != StatusOK {
		t.Fatalf("body status = %q, want %q", body.Status, StatusOK)
	}
}

func TestReadinessNoChecksIsOK(t *testing.T) {
	srv := newServer(t)

	code, body := get(t, srv.URL+DefaultReadinessPath)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d", code, http.StatusOK)
	}
	if body.Status != StatusOK {
		t.Fatalf("body status = %q, want %q", body.Status, StatusOK)
	}
}

func TestReadinessPassingCheck(t *testing.T) {
	srv := newServer(t, WithReadinessCheck("db", func(context.Context) error {
		return nil
	}))

	code, body := get(t, srv.URL+DefaultReadinessPath)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d", code, http.StatusOK)
	}
	if len(body.Failures) != 0 {
		t.Fatalf("failures = %v, want none", body.Failures)
	}
}

func TestReadinessFailingCheck(t *testing.T) {
	srv := newServer(t,
		WithReadinessCheck("db", func(context.Context) error {
			return errors.New("connection refused")
		}),
		WithReadinessCheck("cache", func(context.Context) error {
			return nil
		}),
	)

	code, body := get(t, srv.URL+DefaultReadinessPath)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", code, http.StatusServiceUnavailable)
	}
	if body.Status != StatusUnavailable {
		t.Fatalf("body status = %q, want %q", body.Status, StatusUnavailable)
	}
	if got := body.Failures["db"]; got != "connection refused" {
		t.Fatalf("db failure = %q, want %q", got, "connection refused")
	}
	if _, ok := body.Failures["cache"]; ok {
		t.Fatalf("cache should not be reported as failed: %v", body.Failures)
	}
}

func TestLivenessFailingCheck(t *testing.T) {
	srv := newServer(t, WithLivenessCheck("deadlock", func(context.Context) error {
		return errors.New("stalled")
	}))

	code, body := get(t, srv.URL+DefaultLivenessPath)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", code, http.StatusServiceUnavailable)
	}
	if body.Failures["deadlock"] != "stalled" {
		t.Fatalf("failures = %v, want deadlock:stalled", body.Failures)
	}
}

func TestCustomPaths(t *testing.T) {
	srv := newServer(t,
		WithHealthPath("/health"),
		WithReadinessPath("/ready"),
		WithLivenessPath("/live"),
	)

	for _, path := range []string{"/health", "/ready", "/live"} {
		code, _ := get(t, srv.URL+path)
		if code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want %d", path, code, http.StatusOK)
		}
	}

	// Default paths should no longer be registered.
	resp, err := http.Get(srv.URL + DefaultHealthPath)
	if err != nil {
		t.Fatalf("GET default health: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("default path status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestNonGETRejected(t *testing.T) {
	srv := newServer(t)

	resp, err := http.Post(srv.URL+DefaultHealthPath, "text/plain", http.NoBody)
	if err != nil {
		t.Fatalf("POST %s: %v", DefaultHealthPath, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}

func TestReadinessCheckReceivesContext(t *testing.T) {
	var gotCtx bool
	srv := newServer(t, WithReadinessCheck("ctx", func(ctx context.Context) error {
		gotCtx = ctx != nil
		return nil
	}))

	get(t, srv.URL+DefaultReadinessPath)
	if !gotCtx {
		t.Fatal("check did not receive a non-nil context")
	}
}
