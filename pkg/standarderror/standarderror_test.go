package standarderror

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestNewWrapsCauseAndMessage(t *testing.T) {
	cause := errors.New("boom")
	err := New(Params{Err: cause, Message: "load failed"})

	if got, want := err.Error(), "load failed: boom"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(err, cause) {
		t.Fatal("errors.Is should find the wrapped cause")
	}
}

func TestNewNoCauseUsesMessageOnly(t *testing.T) {
	err := New(Params{Message: "standalone"})
	if got := err.Error(); got != "standalone" {
		t.Fatalf("Error() = %q, want %q", got, "standalone")
	}
	if err.Unwrap() != nil {
		t.Fatal("Unwrap should be nil when there is no cause")
	}
}

func TestNewEmptyMessageWithCauseRendersCause(t *testing.T) {
	cause := errors.New("boom")
	err := New(Params{Err: cause})
	if got := err.Error(); got != "boom" {
		t.Fatalf("Error() = %q, want %q", got, "boom")
	}
}

func TestCodeAndStatus(t *testing.T) {
	err := New(Params{
		Message: "nope",
		Code:    "USER_NOT_FOUND",
		Status:  http.StatusNotFound,
	})
	if got := err.Code(); got != "USER_NOT_FOUND" {
		t.Fatalf("Code() = %q, want %q", got, "USER_NOT_FOUND")
	}
	if got := err.Status(); got != http.StatusNotFound {
		t.Fatalf("Status() = %d, want %d", got, http.StatusNotFound)
	}
}

func TestInvalidStatusIgnored(t *testing.T) {
	err := New(Params{Message: "x", Status: 42})
	if got := err.Status(); got != 0 {
		t.Fatalf("Status() = %d, want 0 for out-of-range input", got)
	}
}

func TestCodeAndStatusInheritedFromCause(t *testing.T) {
	inner := New(Params{
		Message: "inner",
		Code:    "INNER",
		Status:  http.StatusBadRequest,
	})
	outer := New(Params{Err: inner, Message: "outer"})

	if got := outer.Code(); got != "INNER" {
		t.Fatalf("Code() = %q, want inherited %q", got, "INNER")
	}
	if got := outer.Status(); got != http.StatusBadRequest {
		t.Fatalf("Status() = %d, want inherited %d", got, http.StatusBadRequest)
	}
}

func TestOuterCodeAndStatusWin(t *testing.T) {
	inner := New(Params{Message: "inner", Code: "INNER", Status: 400})
	outer := New(Params{Err: inner, Message: "outer", Code: "OUTER", Status: 409})

	if got := outer.Code(); got != "OUTER" {
		t.Fatalf("Code() = %q, want %q", got, "OUTER")
	}
	if got := outer.Status(); got != 409 {
		t.Fatalf("Status() = %d, want %d", got, 409)
	}
}

func TestStackTraceCaptured(t *testing.T) {
	err := New(Params{Message: "trace me"})
	frames := err.StackTrace()
	if len(frames) == 0 {
		t.Fatal("expected at least one captured frame")
	}
	// The first frame should be this test function, not an internal frame.
	if !strings.Contains(frames[0].Function, "TestStackTraceCaptured") {
		t.Fatalf("first frame = %q, want it to be the caller of New", frames[0].Function)
	}
}

func TestStackTraceReusedFromCause(t *testing.T) {
	inner := New(Params{Message: "inner"})
	innerTop := inner.StackTrace()[0]

	outer := New(Params{Err: inner, Message: "outer"})
	outerTop := outer.StackTrace()[0]

	if innerTop != outerTop {
		t.Fatalf("outer trace top = %+v, want reused inner top %+v", outerTop, innerTop)
	}
}

func TestPackageHelpers(t *testing.T) {
	plain := errors.New("plain")
	if Code(plain) != "" || Status(plain) != 0 || StackTrace(plain) != nil {
		t.Fatal("helpers should return zero values for a non-standard error")
	}

	err := New(Params{Err: plain, Message: "wrapped", Code: "C", Status: 503})
	if Code(err) != "C" {
		t.Fatalf("Code() = %q, want %q", Code(err), "C")
	}
	if Status(err) != 503 {
		t.Fatalf("Status() = %d, want 503", Status(err))
	}
	if len(StackTrace(err)) == 0 {
		t.Fatal("StackTrace() should return frames for a standard error")
	}
}

func TestErrorsAsFindsStandardError(t *testing.T) {
	err := New(Params{Err: errors.New("x"), Message: "y", Code: "Z"})
	var se *Error
	if !errors.As(err, &se) {
		t.Fatal("errors.As should find *Error")
	}
	if se.Code() != "Z" {
		t.Fatalf("Code() = %q, want %q", se.Code(), "Z")
	}
}
