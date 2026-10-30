package commands

// Purpose: keep the stderr of the `nself build` child that deploy spawns, so a
//          failed build is diagnosable from the error alone. Over MCP or HTTP
//          the server's own stderr is never shown to the caller, so before this
//          the caller saw only "exit status 1".
// Inputs:  the child's stderr stream.
// Outputs: childStderrTail (an io.Writer) and childFailure (an error that
//          carries the redacted, bounded tail).
// Constraints: only the last childStderrLines non-empty lines, each cut to
//              childStderrLineMax bytes, are kept; every kept line goes through
//              output.RedactText, the redactor error messages use, before it
//              can reach an error or the JSON envelope.

import (
	"strings"
	"sync"

	"github.com/nself-org/cli/internal/output"
)

const (
	childStderrLines   = 25
	childStderrLineMax = 400
	childStderrKeep    = 64 * 1024 // bytes held while collecting
)

// childStderrTail collects the end of a child's stderr.
type childStderrTail struct {
	mu  sync.Mutex
	buf []byte
}

// Write keeps the last childStderrKeep bytes. It never fails, so the child is
// never slowed or broken by the capture.
func (t *childStderrTail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > childStderrKeep {
		t.buf = append([]byte(nil), t.buf[len(t.buf)-childStderrKeep:]...)
	}
	return len(p), nil
}

// String is the redacted tail: the last non-empty lines joined with " | ".
func (t *childStderrTail) String() string {
	t.mu.Lock()
	raw := string(t.buf)
	t.mu.Unlock()
	var lines []string
	for _, l := range strings.Split(raw, "\n") {
		l = strings.TrimSpace(strings.TrimRight(l, "\r"))
		if l == "" {
			continue
		}
		if len(l) > childStderrLineMax {
			l = l[:childStderrLineMax] + "..."
		}
		lines = append(lines, output.RedactText(l))
	}
	if len(lines) > childStderrLines {
		lines = lines[len(lines)-childStderrLines:]
	}
	return strings.Join(lines, " | ")
}

// childFailure is a child process failure that carries the child's stderr tail.
type childFailure struct {
	err  error
	tail string
}

func (f *childFailure) Error() string {
	if f.tail == "" {
		return f.err.Error()
	}
	return f.err.Error() + "; child stderr: " + f.tail
}

func (f *childFailure) Unwrap() error { return f.err }
