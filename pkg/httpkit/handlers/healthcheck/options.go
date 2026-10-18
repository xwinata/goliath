package healthcheck

import "context"

// Check reports whether a dependency is healthy. It returns nil when the
// dependency is ready and a non-nil error describing the problem otherwise. The
// context carries the request deadline, so a Check should respect cancellation.
type Check func(ctx context.Context) error

// Config customizes a Handler created by New. The zero value is valid: empty
// paths fall back to their package defaults, and nil check maps mean the
// corresponding probe simply confirms the process is up.
type Config struct {
	// HealthPath is the path served for the simple health check. Empty selects
	// defaultHealthPath.
	HealthPath string

	// ReadinessPath is the path served for the readiness probe. Empty selects
	// defaultReadinessPath.
	ReadinessPath string

	// LivenessPath is the path served for the liveness probe. Empty selects
	// defaultLivenessPath.
	LivenessPath string

	// ReadinessChecks are the named checks run by the readiness probe, which
	// reports ready only when every check returns nil. Entries with an empty
	// name or nil Check are ignored.
	ReadinessChecks map[string]Check

	// LivenessChecks are the named checks run by the liveness probe, which
	// reports alive only when every check returns nil. Entries with an empty
	// name or nil Check are ignored.
	LivenessChecks map[string]Check
}

// resolvedConfig holds the effective settings after defaults are applied.
type resolvedConfig struct {
	healthPath    string
	readinessPath string
	livenessPath  string
	readiness     map[string]Check
	liveness      map[string]Check
}

// resolved returns c with empty paths replaced by their defaults and the check
// maps copied with empty-name/nil-Check entries dropped, so the Handler never
// shares or mutates the caller's maps.
func (c Config) resolved() resolvedConfig {
	r := resolvedConfig{
		healthPath:    c.HealthPath,
		readinessPath: c.ReadinessPath,
		livenessPath:  c.LivenessPath,
		readiness:     sanitizeChecks(c.ReadinessChecks),
		liveness:      sanitizeChecks(c.LivenessChecks),
	}
	if r.healthPath == "" {
		r.healthPath = defaultHealthPath
	}
	if r.readinessPath == "" {
		r.readinessPath = defaultReadinessPath
	}
	if r.livenessPath == "" {
		r.livenessPath = defaultLivenessPath
	}
	return r
}

// sanitizeChecks returns a copy of in with empty-name and nil-Check entries
// removed. It returns nil when nothing valid remains, so an empty probe stays
// cheap.
func sanitizeChecks(in map[string]Check) map[string]Check {
	var out map[string]Check
	for name, fn := range in {
		if name == "" || fn == nil {
			continue
		}
		if out == nil {
			out = make(map[string]Check, len(in))
		}
		out[name] = fn
	}
	return out
}
