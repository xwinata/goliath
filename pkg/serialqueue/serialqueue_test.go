package serialqueue

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// eventually polls cond until it is true or the timeout elapses.
func eventually(cond func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

func TestSingleRun(t *testing.T) {
	q := New()

	done := make(chan struct{})
	q.Run(func() { close(done) })

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("job did not execute within expected time")
	}
}

func TestEmptyRunRejected(t *testing.T) {
	q := New()
	if got := q.Run(); got != Rejected {
		t.Fatalf("empty Run: got %v, want %v", got, Rejected)
	}
}

func TestVariadicJobsRunInOrder(t *testing.T) {
	q := New()

	var mu sync.Mutex
	var log []string
	done := make(chan struct{})

	q.Run(
		func() { mu.Lock(); log = append(log, "a"); mu.Unlock() },
		func() { mu.Lock(); log = append(log, "b"); mu.Unlock() },
		func() { mu.Lock(); log = append(log, "c"); mu.Unlock(); close(done) },
	)

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("batch did not finish in time")
	}

	mu.Lock()
	defer mu.Unlock()
	want := []string{"a", "b", "c"}
	if len(log) != len(want) {
		t.Fatalf("got %v, want %v", log, want)
	}
	for i := range want {
		if log[i] != want[i] {
			t.Errorf("log[%d]: got %q, want %q", i, log[i], want[i])
		}
	}
}

func TestSecondWaitsUntilFirstFinishes(t *testing.T) {
	var log []string
	var mu sync.Mutex

	q := New()

	job1Started := make(chan struct{})
	job1Finished := make(chan struct{})
	job2Started := make(chan struct{})
	job2Finished := make(chan struct{})

	job1 := func() {
		mu.Lock()
		log = append(log, "job1 start")
		mu.Unlock()
		close(job1Started)
		time.Sleep(300 * time.Millisecond)
		mu.Lock()
		log = append(log, "job1 end")
		mu.Unlock()
		close(job1Finished)
	}

	job2 := func() {
		mu.Lock()
		log = append(log, "job2 start")
		mu.Unlock()
		close(job2Started)
		time.Sleep(100 * time.Millisecond)
		mu.Lock()
		log = append(log, "job2 end")
		mu.Unlock()
		close(job2Finished)
	}

	q.Run(job1)

	select {
	case <-job1Started:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("job1 didn't start within expected time")
	}

	q.Run(job2)

	select {
	case <-job2Started:
		t.Fatal("job2 started before job1 finished")
	case <-time.After(50 * time.Millisecond):
	}

	select {
	case <-job1Finished:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("job1 didn't finish within expected time")
	}

	select {
	case <-job2Started:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("job2 didn't start after job1 finished")
	}

	select {
	case <-job2Finished:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("job2 didn't finish within expected time")
	}

	expected := []string{"job1 start", "job1 end", "job2 start", "job2 end"}

	mu.Lock()
	actualLog := make([]string, len(log))
	copy(actualLog, log)
	mu.Unlock()

	if len(actualLog) != len(expected) {
		t.Fatalf("unexpected log length: got %d entries %v, want %d entries %v",
			len(actualLog), actualLog, len(expected), expected)
	}
	for i := range expected {
		if actualLog[i] != expected[i] {
			t.Errorf("log[%d]: got %q, want %q", i, actualLog[i], expected[i])
		}
	}
}

func TestThirdIsRejectedIfSecondIsWaiting(t *testing.T) {
	var log []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	q := New(WithCapacity(1))

	job1Started := make(chan struct{})
	job2Submitted := make(chan struct{})

	job1 := func() {
		mu.Lock()
		log = append(log, "job1 start")
		mu.Unlock()
		close(job1Started)
		time.Sleep(200 * time.Millisecond)
		mu.Lock()
		log = append(log, "job1 end")
		mu.Unlock()
		wg.Done()
	}

	job2 := func() {
		mu.Lock()
		log = append(log, "job2 start")
		mu.Unlock()
		time.Sleep(100 * time.Millisecond)
		mu.Lock()
		log = append(log, "job2 end")
		mu.Unlock()
		wg.Done()
	}

	job3 := func() {
		mu.Lock()
		log = append(log, "job3 start")
		mu.Unlock()
		wg.Done() // never expected
	}

	wg.Add(2)

	if result1 := q.Run(job1); result1 != Accepted {
		t.Fatal("job1 should have been accepted")
	}

	select {
	case <-job1Started:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("job1 didn't start")
	}

	go func() {
		if result2 := q.Run(job2); result2 != Accepted {
			t.Error("job2 should have been accepted")
		}
		close(job2Submitted)
	}()

	select {
	case <-job2Submitted:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("job2 wasn't submitted")
	}

	if !q.IsRunning() {
		t.Error("queue should be running job1")
	}
	if !q.IsWaiting() {
		t.Error("queue should have job2 waiting")
	}

	if result3 := q.Run(job3); result3 != Rejected {
		t.Error("job3 should have been rejected")
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("test timed out waiting for jobs to complete")
	}

	if q.IsRunning() {
		t.Error("queue should not be running after jobs complete")
	}
	if q.IsWaiting() {
		t.Error("queue should not be waiting after jobs complete")
	}

	mu.Lock()
	actualLog := make([]string, len(log))
	copy(actualLog, log)
	mu.Unlock()

	expected := []string{"job1 start", "job1 end", "job2 start", "job2 end"}
	if len(actualLog) != len(expected) {
		t.Fatalf("unexpected log length: got %d entries %v, want %d entries %v",
			len(actualLog), actualLog, len(expected), expected)
	}
	for i := range expected {
		if actualLog[i] != expected[i] {
			t.Errorf("log[%d]: got %q, want %q", i, actualLog[i], expected[i])
		}
	}
	for _, entry := range actualLog {
		if strings.Contains(entry, "job3") {
			t.Error("job3 should have been rejected but appears in log")
		}
	}
}

func TestTimeoutKillsWaitingJob(t *testing.T) {
	var logMu sync.Mutex
	var log []string

	job1Started := make(chan struct{})
	job1Finished := make(chan struct{})
	job2Submitted := make(chan struct{})
	job2Settled := make(chan struct{})

	job1 := func() {
		logMu.Lock()
		log = append(log, "job1 start")
		logMu.Unlock()
		close(job1Started)
		time.Sleep(500 * time.Millisecond) // longer than timeout
		logMu.Lock()
		log = append(log, "job1 end")
		logMu.Unlock()
		close(job1Finished)
	}

	job2 := func() {
		logMu.Lock()
		log = append(log, "job2 start")
		logMu.Unlock()
		time.Sleep(100 * time.Millisecond)
		logMu.Lock()
		log = append(log, "job2 end")
		logMu.Unlock()
	}

	q := New(WithTimeout(200 * time.Millisecond))

	if result1 := q.Run(job1); result1 != Accepted {
		t.Fatal("job1 should have been accepted")
	}

	select {
	case <-job1Started:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("job1 didn't start within expected time")
	}

	go func() {
		if result2 := q.Run(job2); result2 != Accepted {
			t.Error("job2 should have been accepted (to wait)")
		}
		close(job2Submitted)
		close(job2Settled)
	}()

	select {
	case <-job2Submitted:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("job2 wasn't submitted within expected time")
	}

	if !q.IsRunning() {
		t.Error("queue should be running job1")
	}
	if !q.IsWaiting() {
		t.Error("queue should have job2 waiting")
	}

	select {
	case <-job1Finished:
	case <-time.After(700 * time.Millisecond):
		t.Fatal("job1 didn't finish within expected time")
	}

	select {
	case <-job2Settled:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("job2 should have settled (timed out) by now")
	}

	time.Sleep(50 * time.Millisecond)

	if q.IsRunning() {
		t.Error("queue should not be running after timeout")
	}
	if q.IsWaiting() {
		t.Error("queue should not be waiting after timeout")
	}

	logMu.Lock()
	actualLog := make([]string, len(log))
	copy(actualLog, log)
	logMu.Unlock()

	expected := []string{"job1 start", "job1 end"}
	if len(actualLog) != len(expected) {
		t.Errorf("unexpected log length: got %d entries %v, want %d entries %v",
			len(actualLog), actualLog, len(expected), expected)
	}
	for _, entry := range actualLog {
		if strings.Contains(entry, "job2") {
			t.Error("job2 should have timed out and never executed")
		}
	}
}

func TestContextCancelKillsWaitingJob(t *testing.T) {
	q := New()

	job1Started := make(chan struct{})
	job1Release := make(chan struct{})
	job2Ran := make(chan struct{})

	q.Run(func() {
		close(job1Started)
		<-job1Release
	})

	<-job1Started

	ctx, cancel := context.WithCancel(context.Background())
	settled := make(chan struct{})
	go func() {
		if got := q.RunContext(ctx, func() { close(job2Ran) }); got != Accepted {
			t.Errorf("job2 should have been accepted to wait, got %v", got)
		}
		close(settled)
	}()

	// Let job2 take the waiting slot, then cancel before job1 finishes.
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-settled

	// The waiting goroutine clears the slot as it unwinds; give it a moment.
	if !eventually(func() bool { return !q.IsWaiting() }, 500*time.Millisecond) {
		t.Error("waiting slot should be cleared after cancellation")
	}

	// Release job1 and make sure job2 never runs.
	close(job1Release)
	select {
	case <-job2Ran:
		t.Fatal("job2 ran despite context cancellation")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestPanicRecovered(t *testing.T) {
	var got any
	var mu sync.Mutex
	handled := make(chan struct{})

	q := New(WithPanicHandler(func(r any) {
		mu.Lock()
		got = r
		mu.Unlock()
		close(handled)
	}))

	q.Run(func() { panic("boom") })

	select {
	case <-handled:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("panic handler was not invoked")
	}

	mu.Lock()
	if got != "boom" {
		t.Errorf("recovered value: got %v, want %q", got, "boom")
	}
	mu.Unlock()

	// Queue must remain usable after a panic.
	done := make(chan struct{})
	q.Run(func() { close(done) })
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("queue unusable after a panicking job")
	}
	if q.IsRunning() || q.IsWaiting() {
		t.Error("queue should be idle after recovery")
	}
}

func TestResultString(t *testing.T) {
	if Accepted.String() != "Accepted" || Rejected.String() != "Rejected" {
		t.Fatalf("unexpected Result strings: %q, %q", Accepted, Rejected)
	}
}

func TestCronLikeScenario(t *testing.T) {
	var logMu sync.Mutex
	var log []string

	addToLog := func(entry string) {
		logMu.Lock()
		log = append(log, entry)
		logMu.Unlock()
	}

	var counterMu sync.Mutex
	jobCounter := 0
	createJob := func() func() {
		counterMu.Lock()
		jobCounter++
		jobID := jobCounter
		counterMu.Unlock()
		return func() {
			addToLog(fmt.Sprintf("job%d start", jobID))
			time.Sleep(600 * time.Millisecond)
			addToLog(fmt.Sprintf("job%d fin", jobID))
		}
	}

	q := New(WithCapacity(1), WithTimeout(300*time.Millisecond))

	var results []Result
	var wg sync.WaitGroup
	var resultsMu sync.Mutex

	submitJob := func(delay time.Duration) {
		time.Sleep(delay)
		result := q.Run(createJob())
		resultsMu.Lock()
		results = append(results, result)
		resultsMu.Unlock()
		wg.Done()
	}

	wg.Add(6)
	go submitJob(0)
	go submitJob(100 * time.Millisecond)
	go submitJob(200 * time.Millisecond)
	go submitJob(300 * time.Millisecond)
	go submitJob(400 * time.Millisecond)
	go submitJob(800 * time.Millisecond)

	wg.Wait()
	time.Sleep(2 * time.Second)

	logMu.Lock()
	actualLog := make([]string, len(log))
	copy(actualLog, log)
	logMu.Unlock()

	t.Logf("actual log: %v", actualLog)

	expected := []string{"job1 start", "job1 fin", "job6 start", "job6 fin"}
	if len(actualLog) != len(expected) {
		t.Errorf("unexpected log length: got %d entries %v, want %d entries %v",
			len(actualLog), actualLog, len(expected), expected)
	}
	for _, entry := range actualLog {
		if strings.Contains(entry, "job2") {
			t.Error("job2 should have timed out and never executed")
		}
	}

	if q.IsRunning() {
		t.Error("queue should not be running after all jobs complete")
	}
	if q.IsWaiting() {
		t.Error("queue should not be waiting after timeout")
	}
}

func TestDefaultCapacity(t *testing.T) {
	q := New()

	block := make(chan struct{})
	started := make(chan struct{})

	// Occupy the running slot without freeing it.
	q.Run(func() {
		close(started)
		<-block
	})
	<-started

	// Fill the default number of waiting seats.
	for i := 0; i < DefaultCapacity; i++ {
		if got := q.Run(func() {}); got != Accepted {
			t.Fatalf("submission %d: got %v, want Accepted", i, got)
		}
	}

	if got := q.Waiting(); got != DefaultCapacity {
		t.Errorf("Waiting: got %d, want %d", got, DefaultCapacity)
	}

	// One more must be rejected: running + full backlog.
	if got := q.Run(func() {}); got != Rejected {
		t.Errorf("overflow submission: got %v, want Rejected", got)
	}

	close(block)
}

func TestCustomCapacityRejectsWhenFull(t *testing.T) {
	q := New(WithCapacity(2))

	block := make(chan struct{})
	started := make(chan struct{})

	q.Run(func() {
		close(started)
		<-block
	})
	<-started

	if got := q.Run(func() {}); got != Accepted {
		t.Fatalf("seat 1: got %v, want Accepted", got)
	}
	if got := q.Run(func() {}); got != Accepted {
		t.Fatalf("seat 2: got %v, want Accepted", got)
	}
	if got := q.Run(func() {}); got != Rejected {
		t.Errorf("seat 3: got %v, want Rejected", got)
	}

	close(block)
}

func TestCapacityClampedToOne(t *testing.T) {
	q := New(WithCapacity(0)) // clamped to 1

	block := make(chan struct{})
	started := make(chan struct{})

	q.Run(func() {
		close(started)
		<-block
	})
	<-started

	if got := q.Run(func() {}); got != Accepted {
		t.Fatalf("first waiter: got %v, want Accepted", got)
	}
	if got := q.Run(func() {}); got != Rejected {
		t.Errorf("second waiter: got %v, want Rejected", got)
	}

	close(block)
}

func TestBacklogRunsInFIFOOrder(t *testing.T) {
	q := New(WithCapacity(5))

	var mu sync.Mutex
	var order []int
	block := make(chan struct{})
	started := make(chan struct{})
	done := make(chan struct{})

	// Hold the running slot so the rest queue up in order.
	q.Run(func() {
		close(started)
		<-block
	})
	<-started

	const n = 4
	for i := 0; i < n; i++ {
		id := i
		last := i == n-1
		q.Run(func() {
			mu.Lock()
			order = append(order, id)
			mu.Unlock()
			if last {
				close(done)
			}
		})
	}

	close(block)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("backlog did not drain in time")
	}

	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < n; i++ {
		if order[i] != i {
			t.Errorf("order[%d] = %d, want %d (got %v)", i, order[i], i, order)
			break
		}
	}
}
