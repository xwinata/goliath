package healthcheck

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newServer builds a test server with the handler mounted on a fresh mux.
func newServer(t *testing.T, cfg Config) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	New(cfg).Register(mux)
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
	srv := newServer(t, Config{})

	code, body := get(t, srv.URL+defaultHealthPath)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d", code, http.StatusOK)
	}
	if body.Status != StatusOK {
		t.Fatalf("body status = %q, want %q", body.Status, StatusOK)
	}
}

func TestReadinessNoChecksIsOK(t *testing.T) {
	srv := newServer(t, Config{})

	code, body := get(t, srv.URL+defaultReadinessPath)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d", code, http.StatusOK)
	}
	if body.Status != StatusOK {
		t.Fatalf("body status = %q, want %q", body.Status, StatusOK)
	}
}

func TestReadinessPassingCheck(t *testing.T) {
	srv := newServer(t, Config{
		ReadinessChecks: map[string]Check{
			"db": func(context.Context) error { return nil },
		},
	})

	code, body := get(t, srv.URL+defaultReadinessPath)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d", code, http.StatusOK)
	}
	if len(body.Failures) != 0 {
		t.Fatalf("failures = %v, want none", body.Failures)
	}
}

func TestReadinessFailingCheck(t *testing.T) {
	srv := newServer(t, Config{
		ReadinessChecks: map[string]Check{
			"db": func(context.Context) error {
				return errors.New("connection refused")
			},
			"cache": func(context.Context) error { return nil },
		},
	})

	code, body := get(t, srv.URL+defaultReadinessPath)
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
	srv := newServer(t, Config{
		LivenessChecks: map[string]Check{
			"deadlock": func(context.Context) error {
				return errors.New("stalled")
			},
		},
	})

	code, body := get(t, srv.URL+defaultLivenessPath)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", code, http.StatusServiceUnavailable)
	}
	if body.Failures["deadlock"] != "stalled" {
		t.Fatalf("failures = %v, want deadlock:stalled", body.Failures)
	}
}

func TestCustomPaths(t *testing.T) {
	srv := newServer(t, Config{
		HealthPath:    "/health",
		ReadinessPath: "/ready",
		LivenessPath:  "/live",
	})

	for _, path := range []string{"/health", "/ready", "/live"} {
		code, _ := get(t, srv.URL+path)
		if code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want %d", path, code, http.StatusOK)
		}
	}

	// Default paths should no longer be registered.
	resp, err := http.Get(srv.URL + defaultHealthPath)
	if err != nil {
		t.Fatalf("GET default health: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("default path status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestNonGETRejected(t *testing.T) {
	srv := newServer(t, Config{})

	resp, err := http.Post(srv.URL+defaultHealthPath, "text/plain", http.NoBody)
	if err != nil {
		t.Fatalf("POST %s: %v", defaultHealthPath, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}

func TestReadinessCheckReceivesContext(t *testing.T) {
	var gotCtx bool
	srv := newServer(t, Config{
		ReadinessChecks: map[string]Check{
			"ctx": func(ctx context.Context) error {
				gotCtx = ctx != nil
				return nil
			},
		},
	})

	get(t, srv.URL+defaultReadinessPath)
	if !gotCtx {
		t.Fatal("check did not receive a non-nil context")
	}
}
