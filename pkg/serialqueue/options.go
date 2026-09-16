package serialqueue

import "time"

// options holds Queue configuration collected from Option values.
type options struct {
	capacity int
	timeout  time.Duration
	onPanic  func(any)
}

// Option configures a Queue in New.
type Option func(*options)

// WithCapacity sets how many jobs may wait behind the running one before further
// submissions are rejected. The running job does not count against it. Values
// below 1 are clamped to 1. The default is DefaultCapacity.
func WithCapacity(n int) Option {
	return func(o *options) {
		if n < 1 {
			n = 1
		}
		o.capacity = n
	}
}

// WithTimeout bounds how long a queued job waits for the running one to finish.
// If it expires first, the queued job is discarded. Zero (the default) waits
// indefinitely.
func WithTimeout(d time.Duration) Option {
	return func(o *options) {
		o.timeout = d
	}
}

// WithPanicHandler sets a callback invoked with the recovered value when a job
// panics. Without it, a panicking job is recovered silently so the queue keeps
// running.
func WithPanicHandler(fn func(recovered any)) Option {
	return func(o *options) {
		o.onPanic = fn
	}
}
