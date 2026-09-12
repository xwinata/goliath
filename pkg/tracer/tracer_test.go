package tracer

import (
	"runtime"
	"strings"
	"sync"
	"testing"
)

// nested call chain used to assert the captured stack contains each level.
func helperA() []Frame { return helperB() }
func helperB() []Frame { return helperC() }
func helperC() []Frame { return Trace() }

func TestTrace_BasicCapture(t *testing.T) {
	frames := Trace()

	if len(frames) == 0 {
		t.Fatal("expected at least one frame")
	}
	// First frame is the caller of Trace: this test function.
	if !strings.Contains(frames[0].Function, "TestTrace_BasicCapture") {
		t.Errorf("first frame = %q, want the test function", frames[0].Function)
	}
	if frames[0].File == "" {
		t.Error("expected non-empty file")
	}
	if frames[0].Line <= 0 {
		t.Errorf("expected positive line, got %d", frames[0].Line)
	}
	if !strings.HasSuffix(frames[0].File, ".go") {
		t.Errorf("expected .go file, got %q", frames[0].File)
	}
}

func TestTrace_NestedCalls(t *testing.T) {
	frames := helperA()

	var a, b, c bool
	for _, f := range frames {
		switch {
		case strings.Contains(f.Function, "helperC"):
			c = true
		case strings.Contains(f.Function, "helperB"):
			b = true
		case strings.Contains(f.Function, "helperA"):
			a = true
		}
	}
	if !a || !b || !c {
		t.Errorf("expected helperA/B/C in stack; got a=%v b=%v c=%v", a, b, c)
	}
}

func TestTrace_WithMaxDepth(t *testing.T) {
	frames := Trace(WithMaxDepth(3))
	if len(frames) == 0 {
		t.Fatal("expected at least one frame")
	}
	if len(frames) > 3 {
		t.Errorf("expected at most 3 frames, got %d", len(frames))
	}
}

func TestTrace_WithSkipIncludesTraceItself(t *testing.T) {
	// Skipping only 1 frame means runtime.Callers' caller (Trace) is included.
	frames := Trace(WithSkip(1))

	found := false
	for _, f := range frames {
		if strings.Contains(f.Function, "tracer.Trace") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Trace itself in the stack when skip=1")
	}
}

func TestTrace_HighSkipReturnsFew(t *testing.T) {
	frames := Trace(WithSkip(1000))
	if len(frames) > 2 {
		t.Errorf("expected very few frames with high skip, got %d", len(frames))
	}
}

func TestTrace_WithFilter(t *testing.T) {
	frames := Trace(WithFilter(func(f *runtime.Frame) bool {
		return strings.Contains(f.Function, "tracer.Test")
	}))

	if len(frames) == 0 {
		t.Fatal("expected at least one filtered frame")
	}
	for _, f := range frames {
		if !strings.Contains(f.Function, "tracer.Test") {
			t.Errorf("frame %q did not match filter", f.Function)
		}
	}
}

func TestTrace_FilterRejectingAllReturnsEmpty(t *testing.T) {
	frames := Trace(WithFilter(func(*runtime.Frame) bool { return false }))
	if len(frames) != 0 {
		t.Errorf("expected no frames when filter rejects all, got %d", len(frames))
	}
}

func TestTrace_NonPositiveOptionsKeepDefaults(t *testing.T) {
	// skip<=0 and maxDepth<=0 must be ignored, so defaults still capture frames.
	frames := Trace(WithSkip(-5), WithMaxDepth(0))
	if len(frames) == 0 {
		t.Error("expected defaults to apply and capture frames")
	}
}

func TestTrace_ConcurrentSafe(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(20)
	for i := 0; i < 20; i++ {
		go func() {
			defer wg.Done()
			if frames := Trace(); len(frames) == 0 {
				t.Error("expected frames from concurrent Trace")
			}
		}()
	}
	wg.Wait()
}
