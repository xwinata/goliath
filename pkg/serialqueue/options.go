package serialqueue

import "time"

// Config customizes a Queue created by New. The zero value is valid: Capacity
// <= 0 selects DefaultCapacity, a zero Timeout waits indefinitely, and a nil
// PanicHandler recovers panicking jobs silently.
type Config struct {
	// Capacity is how many jobs may wait behind the running one before further
	// submissions are rejected. The running job does not count against it.
	// Values below 1 are clamped to 1; zero selects DefaultCapacity.
	Capacity int

	// Timeout bounds how long a queued job waits for the running one to finish.
	// If it expires first, the queued job is discarded. Zero waits
	// indefinitely.
	Timeout time.Duration

	// PanicHandler, when non-nil, is invoked with the recovered value when a
	// job panics. Without it, a panicking job is recovered silently so the
	// queue keeps running.
	PanicHandler func(recovered any)
}

// resolvedConfig holds the effective settings after defaults are applied.
type resolvedConfig struct {
	capacity int
	timeout  time.Duration
	onPanic  func(any)
}

// resolved returns c with defaults and clamping applied: Capacity zero becomes
// DefaultCapacity and any value below 1 is clamped to 1.
func (c Config) resolved() resolvedConfig {
	capacity := c.Capacity
	switch {
	case capacity == 0:
		capacity = DefaultCapacity
	case capacity < 1:
		capacity = 1
	}
	return resolvedConfig{
		capacity: capacity,
		timeout:  c.Timeout,
		onPanic:  c.PanicHandler,
	}
}
