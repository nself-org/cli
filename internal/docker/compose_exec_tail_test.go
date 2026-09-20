package docker

// Purpose: cover the bounded stderr tail that Run wraps into its error.
//
// Inputs:  byte writes, possibly exceeding the cap.
// Outputs: assertions on retained content and size.
// Constraints: in non-TTY contexts (all of CI) Run sends subprocess stderr to
// io.Discard so goroutines never touch os.Stdout after the process exits. That
// also threw away docker's actual message, leaving errors that read only
// "exit status 1". The tail restores the diagnosis without reintroducing the
// fd-lifetime problem, because it is an in-memory buffer, not a real sink.

import (
	"strings"
	"testing"
)

func TestTailBuffer_KeepsShortOutputWhole(t *testing.T) {
	tb := &tailBuffer{limit: 2048}
	tb.Write([]byte("no such service: admin")) //nolint:errcheck

	if got, want := tb.String(), "no such service: admin"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestTailBuffer_CollapsesWhitespaceToOneLine(t *testing.T) {
	tb := &tailBuffer{limit: 2048}
	tb.Write([]byte("Error response from daemon:\n  No such container\n\n")) //nolint:errcheck

	got := tb.String()
	if strings.Contains(got, "\n") {
		t.Errorf("String() must not contain newlines, got %q", got)
	}
	if want := "Error response from daemon: No such container"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestTailBuffer_RetainsTailNotHead(t *testing.T) {
	// Docker puts the actionable line last, so an over-long stream must keep
	// the END. Keeping the head would discard exactly the useful part.
	tb := &tailBuffer{limit: 32}
	tb.Write([]byte(strings.Repeat("x", 100))) //nolint:errcheck
	tb.Write([]byte("no such service: admin")) //nolint:errcheck

	got := tb.String()
	if !strings.HasSuffix(got, "no such service: admin") {
		t.Errorf("tail lost the final message, got %q", got)
	}
	if len(got) > 32 {
		t.Errorf("retained %d bytes, limit is 32: %q", len(got), got)
	}
}

func TestTailBuffer_BoundedAcrossManyWrites(t *testing.T) {
	// A chatty failure must not grow memory without bound.
	tb := &tailBuffer{limit: 64}
	for i := 0; i < 1000; i++ {
		tb.Write([]byte("chatty docker progress line ")) //nolint:errcheck
	}

	if len(tb.buf) > 64 {
		t.Errorf("buffer grew to %d bytes, limit is 64", len(tb.buf))
	}
}

func TestTailBuffer_EmptyStaysEmpty(t *testing.T) {
	// Run only appends a detail suffix when the tail is non-empty; a silent
	// failure must not produce a dangling ": " on the error.
	tb := &tailBuffer{limit: 2048}

	if got := tb.String(); got != "" {
		t.Errorf("String() = %q, want empty", got)
	}
}

func TestTailBuffer_WriteReportsFullLength(t *testing.T) {
	// io.MultiWriter treats a short write as an error and would abort the
	// stderr copy, so Write must always report len(p) even when it drops
	// bytes past the cap.
	tb := &tailBuffer{limit: 4}

	n, err := tb.Write([]byte("much longer than four"))
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if want := len("much longer than four"); n != want {
		t.Errorf("Write returned n=%d, want %d (io.MultiWriter aborts on short writes)", n, want)
	}
}
