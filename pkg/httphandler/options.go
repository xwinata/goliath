package httphandler

import "context"

// Check reports whether a dependency is healthy. It returns nil when the
// dependency is ready and a non-nil error describing the problem otherwise. The
// context carries the request deadline, so a Check should respect cancellation.
type Check func(ctx context.Context) error

// options holds Handler configuration collected from Option values.
type options struct {
	healthPath    string
	readinessPath string
	livenessPath  string
	readiness     map[string]Check
	liveness      map[string]Check
}

// Option configures a Handler in New.
type Option func(*options)

// WithHealthPath overrides the path served for the simple health check. Empty
// values are ignored. The default is DefaultHealthPath.
func WithHealthPath(path string) Option {
	return func(o *options) {
		if path != "" {
			o.healthPath = path
		}
	}
}

// WithReadinessPath overrides the path served for the readiness probe. Empty
// values are ignored. The default is DefaultReadinessPath.
func WithReadinessPath(path string) Option {
	return func(o *options) {
		if path != "" {
			o.readinessPath = path
		}
	}
}

// WithLivenessPath overrides the path served for the liveness probe. Empty
// values are ignored. The default is DefaultLivenessPath.
func WithLivenessPath(path string) Option {
	return func(o *options) {
		if path != "" {
			o.livenessPath = path
		}
	}
}

// WithReadinessCheck registers a named check run by the readiness probe. The
// probe reports ready only when every registered check returns nil. Registering
// the same name twice replaces the earlier check. A nil fn is ignored.
func WithReadinessCheck(name string, fn Check) Option {
	return func(o *options) {
		if name == "" || fn == nil {
			return
		}
		if o.readiness == nil {
			o.readiness = make(map[string]Check)
		}
		o.readiness[name] = fn
	}
}

// WithLivenessCheck registers a named check run by the liveness probe. The probe
// reports alive only when every registered check returns nil. With no checks the
// probe simply confirms the process is running. Registering the same name twice
// replaces the earlier check. A nil fn is ignored.
func WithLivenessCheck(name string, fn Check) Option {
	return func(o *options) {
		if name == "" || fn == nil {
			return
		}
		if o.liveness == nil {
			o.liveness = make(map[string]Check)
		}
		o.liveness[name] = fn
	}
}
