# serialqueue

`serialqueue` runs jobs one at a time in submission order. One job runs while
others wait in a bounded FIFO backlog; a job submitted when the backlog is full
is rejected. It suits work that must never overlap, such as a periodic task
whose runs may outlast its interval.

## Behavior

- **Serial execution.** One job runs at any moment.
- **Bounded backlog.** Up to `capacity` jobs wait behind the running one
  (default 5). The running job does not occupy a seat.
- **FIFO.** Waiting jobs run in the order they were submitted.
- **Reject when full.** A job submitted with a full backlog is rejected.
- **Context-aware waiting.** A queued job is dropped if its context is canceled
  or a timeout fires before it starts.
- **Panic-safe.** A panicking job is recovered so the queue keeps running.

## Usage

```go
import "goliath/pkg/serialqueue"

q := serialqueue.New(
    serialqueue.WithCapacity(10),
    serialqueue.WithTimeout(5*time.Second),
)

// Runs immediately.
q.Run(func() {
    doWork()
})

// Queues behind the running job; dropped if it waits past the timeout.
q.Run(func() {
    doMoreWork()
})

// Rejected once the backlog is full.
if q.Run(func() {}) == serialqueue.Rejected {
    // handle backpressure
}
```

### Multiple jobs as one unit

```go
q.Run(step1, step2, step3) // run in order, as a single serialized batch
```

### Bring your own context

```go
ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
defer cancel()

q.RunContext(ctx, func() {
    doWork()
})
```

### React to panics

```go
q := serialqueue.New(serialqueue.WithPanicHandler(func(r any) {
    log.Printf("job panicked: %v", r)
}))
```

## API

- `New(opts ...Option) *Queue` — create a queue.
- `(*Queue) Run(jobs ...func()) Result` — submit a batch; returns `Accepted` or `Rejected`.
- `(*Queue) RunContext(ctx, jobs ...func()) Result` — same, bounded by `ctx`.
- `(*Queue) IsRunning() bool` / `IsWaiting() bool` / `Waiting() int` — inspect state.
- `WithCapacity(n int) Option` — set the number of waiting seats (default `DefaultCapacity` = 5).
- `WithTimeout(d time.Duration) Option` — bound the wait for queued jobs.
- `WithPanicHandler(fn func(any)) Option` — observe recovered panics.
