// Package tracer captures runtime call-stack information for debugging and
// error diagnostics.
//
// Unlike a module-object style API, the entry point is a plain package-level
// function:
//
//	frames := tracer.Trace()
//
// Behavior is tuned with functional options:
//
//	frames := tracer.Trace(
//		tracer.WithSkip(3),
//		tracer.WithMaxDepth(10),
//		tracer.WithFilter(func(f *runtime.Frame) bool {
//			return strings.Contains(f.Function, "myapp")
//		}),
//	)
package tracer

import "runtime"

// Frame is a single stack frame with its file, line, and function name.
type Frame struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Function string `json:"function"`
}

const (
	// defaultSkip skips Trace itself and runtime.Callers so the first captured
	// frame is the caller of Trace.
	defaultSkip = 2

	// defaultMaxDepth caps the number of frames captured by default.
	defaultMaxDepth = 32
)

// config holds resolved Trace settings. It is unexported; callers configure it
// through Option values.
type config struct {
	skip     int
	maxDepth int
	filter   func(*runtime.Frame) bool
}

// Option customizes Trace behavior.
type Option func(*config)

// WithSkip sets how many leading frames to skip. Values <= 0 keep the default,
// which skips Trace itself and runtime.Callers so the first frame is Trace's
// caller. Pass a positive value to skip additional frames (for example, from a
// helper that wraps Trace).
func WithSkip(n int) Option {
	return func(c *config) {
		if n > 0 {
			c.skip = n
		}
	}
}

// WithMaxDepth caps the number of frames captured. Values <= 0 keep the default.
func WithMaxDepth(n int) Option {
	return func(c *config) {
		if n > 0 {
			c.maxDepth = n
		}
	}
}

// WithFilter keeps only frames for which keep returns true. Without a filter,
// all frames are captured.
func WithFilter(keep func(*runtime.Frame) bool) Option {
	return func(c *config) {
		c.filter = keep
	}
}

// Trace captures the current call stack and returns the frames.
//
// By default it skips Trace itself and runtime.Callers (so the first frame is
// the caller of Trace), captures up to 32 frames, and includes every frame.
// Use WithSkip, WithMaxDepth, and WithFilter to change this.
//
// Each call operates on its own stack snapshot and is safe for concurrent use.
func Trace(opts ...Option) []Frame {
	cfg := config{
		skip:     defaultSkip,
		maxDepth: defaultMaxDepth,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	pcs := make([]uintptr, cfg.maxDepth)
	n := runtime.Callers(cfg.skip, pcs)
	if n == 0 {
		return nil
	}

	frames := runtime.CallersFrames(pcs[:n])
	out := make([]Frame, 0, n)
	for {
		frame, more := frames.Next()

		if cfg.filter == nil || cfg.filter(&frame) {
			out = append(out, Frame{
				File:     frame.File,
				Line:     frame.Line,
				Function: frame.Function,
			})
		}

		if !more {
			break
		}
	}

	return out
}
