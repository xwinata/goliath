// Package tracer captures runtime call-stack information for debugging and
// error diagnostics.
//
// The entry point is a plain package-level function that takes a Config:
//
//	frames := tracer.Trace(tracer.Config{})
//
// The zero-value Config captures a sensible default stack. Set fields to tune
// it:
//
//	frames := tracer.Trace(tracer.Config{
//		Skip:     3,
//		MaxDepth: 10,
//		Filter: func(f *runtime.Frame) bool {
//			return strings.Contains(f.Function, "myapp")
//		},
//	})
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

// Config tunes Trace. The zero value is valid: Skip <= 0 keeps the default
// (skip Trace itself and runtime.Callers so the first frame is Trace's caller),
// MaxDepth <= 0 keeps the default cap, and a nil Filter captures every frame.
type Config struct {
	// Skip is how many leading frames to skip. Values <= 0 keep the default.
	// Pass a positive value to skip additional frames (for example, from a
	// helper that wraps Trace).
	Skip int

	// MaxDepth caps the number of frames captured. Values <= 0 keep the
	// default.
	MaxDepth int

	// Filter, when non-nil, keeps only frames for which it returns true.
	Filter func(*runtime.Frame) bool
}

// resolved returns c with non-positive Skip/MaxDepth replaced by their
// defaults. Filter is left as-is (nil means capture everything).
func (c Config) resolved() Config {
	if c.Skip <= 0 {
		c.Skip = defaultSkip
	}
	if c.MaxDepth <= 0 {
		c.MaxDepth = defaultMaxDepth
	}
	return c
}

// Trace captures the current call stack and returns the frames.
//
// With a zero-value Config it skips Trace itself and runtime.Callers (so the
// first frame is the caller of Trace), captures up to 32 frames, and includes
// every frame. Set Config.Skip, Config.MaxDepth, and Config.Filter to change
// this.
//
// Each call operates on its own stack snapshot and is safe for concurrent use.
func Trace(cfg Config) []Frame {
	cfg = cfg.resolved()

	pcs := make([]uintptr, cfg.MaxDepth)
	n := runtime.Callers(cfg.Skip, pcs)
	if n == 0 {
		return nil
	}

	frames := runtime.CallersFrames(pcs[:n])
	out := make([]Frame, 0, n)
	for {
		frame, more := frames.Next()

		if cfg.Filter == nil || cfg.Filter(&frame) {
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
