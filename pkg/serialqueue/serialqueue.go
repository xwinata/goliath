// Package serialqueue runs jobs one at a time in submission order.
//
// One job runs while others wait in a bounded FIFO backlog. A job submitted
// when the backlog is full is rejected. This suits work that must not overlap,
// such as a periodic task whose runs may outlast its interval.
//
// The zero value is not usable; create a Queue with New.
//
//	q := serialqueue.New(serialqueue.WithCapacity(10))
//	q.Run(func() { doWork() })
package serialqueue

import (
	"context"
	"sync"
	"time"
)

// DefaultCapacity is the backlog size used when WithCapacity is not set.
const DefaultCapacity = 5

// Result reports the outcome of submitting a job.
type Result int

const (
	// Accepted means the job started or joined the backlog.
	Accepted Result = iota
	// Rejected means the backlog is full, or the submission was empty.
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

// batch is one submission: a group of jobs run as a unit, plus the context that
// can abandon them while they wait and a cancel func to release its resources.
type batch struct {
	ctx    context.Context
	cancel context.CancelFunc
	jobs   []func()
}

// Queue runs jobs serially with a bounded FIFO backlog.
type Queue struct {
	mu       sync.Mutex
	backlog  []batch
	running  bool // a batch is executing now
	draining bool // the worker goroutine is alive

	capacity int
	timeout  time.Duration // 0 means no default timeout
	onPanic  func(any)
}

// New creates a Queue configured by the given options.
func New(opts ...Option) *Queue {
	cfg := options{capacity: DefaultCapacity}
	for _, opt := range opts {
		opt(&cfg)
	}

	return &Queue{
		capacity: cfg.capacity,
		timeout:  cfg.timeout,
		onPanic:  cfg.onPanic,
	}
}

// Run submits one or more jobs and returns immediately. The jobs run as a single
// unit, in order, once earlier submissions finish. It returns Rejected if the
// backlog is full or jobs is empty. If the queue was created with WithTimeout,
// the wait for each submission is bounded by that timeout.
func (q *Queue) Run(jobs ...func()) Result {
	return q.RunContext(context.Background(), jobs...)
}

// RunContext behaves like Run but also drops the queued jobs if ctx is canceled
// or its deadline passes before they start. A per-queue timeout from WithTimeout
// still applies; whichever fires first wins. Cancellation only affects jobs
// still waiting, never a batch that has already started.
func (q *Queue) RunContext(ctx context.Context, jobs ...func()) Result {
	if len(jobs) == 0 {
		return Rejected
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	// Reclaim seats held by batches that were canceled while waiting, then
	// reject once the backlog (waiting seats) is full. The running batch does
	// not occupy a seat.
	q.prune()
	if len(q.backlog) >= q.capacity {
		return Rejected
	}

	// Start the timeout clock at submission so a batch is dropped if it waits
	// too long, not merely if it runs too long.
	cancel := context.CancelFunc(func() {})
	if q.timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, q.timeout)
	}

	q.backlog = append(q.backlog, batch{ctx: ctx, cancel: cancel, jobs: jobs})

	// Start the worker if it is not already draining the backlog.
	if !q.draining {
		q.draining = true
		go q.worker()
	}
	return Accepted
}

// prune drops waiting batches whose context is already done, releasing their
// seats and cancel funcs. The caller must hold q.mu.
func (q *Queue) prune() {
	kept := q.backlog[:0]
	for _, b := range q.backlog {
		if b.ctx.Err() != nil {
			b.cancel()
			continue
		}
		kept = append(kept, b)
	}
	// Clear the tail so dropped batches are not retained by the backing array.
	for i := len(kept); i < len(q.backlog); i++ {
		q.backlog[i] = batch{}
	}
	q.backlog = kept
}

// worker drains the backlog one batch at a time until it is empty, then exits.
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

// execute runs a batch unless its context is already done, in which case the
// batch is dropped. Each job is guarded against panics so one bad job cannot
// crash the process or stall the queue.
func (q *Queue) execute(b batch) {
	defer b.cancel()

	if b.ctx.Err() != nil {
		return // abandoned while waiting
	}

	for _, job := range b.jobs {
		q.runOne(job)
	}
}

// runOne runs a single job, forwarding any panic to onPanic (if set).
func (q *Queue) runOne(job func()) {
	defer func() {
		if r := recover(); r != nil && q.onPanic != nil {
			q.onPanic(r)
		}
	}()
	job()
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
