// Package serialqueue runs jobs one at a time in submission order.
//
// One job runs while others wait in a bounded FIFO backlog. A job submitted
// when the backlog is full is rejected. This suits work that must not overlap,
// such as a periodic task whose runs may outlast its interval.
//
// The zero value is not usable; create a Queue with New.
//
//	q := serialqueue.New(serialqueue.Config{Capacity: 10})
//	q.Run(func() { doWork() })
//
// Each submission is a single job. To run several as one unit, wrap them:
//
//	q.Run(func() { stepA(); stepB() })
package serialqueue

import (
	"context"
	"sync"
	"time"
)

// DefaultCapacity is the backlog size used when Config.Capacity is zero.
const DefaultCapacity = 5

// Result reports the outcome of submitting a job.
type Result int

const (
	// Accepted means the job started or joined the backlog.
	Accepted Result = iota
	// Rejected means the backlog is full, or the job was nil.
	Rejected
)

// String returns the result name, e.g. "Accepted".
func (r Result) String() string {
	switch r {
	case Accepted:
		return "Accepted"
	case Rejected:
		return "Rejected"
	default:
		return "Unknown"
	}
}

// queued is one submitted job, plus the context that can abandon it while it
// waits and a cancel func to release that context's resources.
type queued struct {
	ctx    context.Context
	cancel context.CancelFunc
	job    func()
}

// Queue runs jobs serially with a bounded FIFO backlog.
type Queue struct {
	mu       sync.Mutex
	backlog  []queued
	running  bool // a job is executing now
	draining bool // the worker goroutine is alive

	capacity int
	timeout  time.Duration // 0 means no default timeout
	onPanic  func(any)
}

// New creates a Queue configured by cfg. A zero-value Config yields a queue
// with DefaultCapacity, no timeout, and silent panic recovery.
func New(cfg Config) *Queue {
	rc := cfg.resolved()

	return &Queue{
		capacity: rc.capacity,
		timeout:  rc.timeout,
		onPanic:  rc.onPanic,
	}
}

// Run submits a job and returns immediately. The job runs once earlier
// submissions finish. It returns Rejected if the backlog is full or job is nil.
// To run several jobs as one unit, wrap them in a single func. If the queue was
// created with a Config.Timeout, the wait is bounded by that timeout.
func (q *Queue) Run(job func()) Result {
	return q.RunContext(context.Background(), job)
}

// RunContext behaves like Run but also drops the job if ctx is canceled or its
// deadline passes before it starts. A per-queue timeout from Config.Timeout
// still
// applies; whichever fires first wins. Cancellation only affects a job still
// waiting, never one that has already started.
func (q *Queue) RunContext(ctx context.Context, job func()) Result {
	if job == nil {
		return Rejected
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	// Reclaim seats held by jobs that were canceled while waiting, then reject
	// once the backlog (waiting seats) is full. The running job does not occupy
	// a seat.
	q.prune()
	if len(q.backlog) >= q.capacity {
		return Rejected
	}

	// Start the timeout clock at submission so a job is dropped if it waits too
	// long, not merely if it runs too long.
	cancel := context.CancelFunc(func() {})
	if q.timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, q.timeout)
	}

	q.backlog = append(q.backlog, queued{ctx: ctx, cancel: cancel, job: job})

	// Start the worker if it is not already draining the backlog.
	if !q.draining {
		q.draining = true
		go q.worker()
	}
	return Accepted
}

// prune drops waiting jobs whose context is already done, releasing their seats
// and cancel funcs. The caller must hold q.mu.
func (q *Queue) prune() {
	kept := q.backlog[:0]
	for _, qd := range q.backlog {
		if qd.ctx.Err() != nil {
			qd.cancel()
			continue
		}
		kept = append(kept, qd)
	}
	// Clear the tail so dropped jobs are not retained by the backing array.
	for i := len(kept); i < len(q.backlog); i++ {
		q.backlog[i] = queued{}
	}
	q.backlog = kept
}

// worker drains the backlog one job at a time until it is empty, then exits.
func (q *Queue) worker() {
	for {
		q.mu.Lock()
		if len(q.backlog) == 0 {
			q.draining = false
			q.mu.Unlock()
			return
		}
		next := q.backlog[0]
		q.backlog = q.backlog[1:]
		q.running = true
		q.mu.Unlock()

		q.execute(next)

		q.mu.Lock()
		q.running = false
		q.mu.Unlock()
	}
}

// execute runs a job unless its context is already done, in which case the job
// is dropped. The job is guarded against panics so one bad job cannot crash the
// process or stall the queue.
func (q *Queue) execute(qd queued) {
	defer qd.cancel()

	if qd.ctx.Err() != nil {
		return // abandoned while waiting
	}

	defer func() {
		if r := recover(); r != nil && q.onPanic != nil {
			q.onPanic(r)
		}
	}()
	qd.job()
}

// IsRunning reports whether a job is currently running.
func (q *Queue) IsRunning() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.running
}

// IsWaiting reports whether at least one live job is queued behind the running
// one.
func (q *Queue) IsWaiting() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.prune()
	return len(q.backlog) > 0
}

// Waiting returns the number of live jobs currently queued behind the running
// one. Batches canceled while waiting are excluded.
func (q *Queue) Waiting() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.prune()
	return len(q.backlog)
}
