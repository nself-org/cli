// Package childerr keeps the stderr of a child process that deploy spawns, so
// a failed `nself build` is diagnosable from the error alone.
//
// Purpose: over MCP or HTTP the server's own stderr is never shown to the
//
//	caller, so a failed child used to surface as "exit status 1".
//
// Inputs:  an *exec.Cmd about to run.
// Outputs: a Failure error that carries the redacted, bounded stderr tail.
// Constraints: only the last MaxLines non-empty lines, each cut to MaxLine
//
//	bytes, are kept; every kept line goes through output.RedactText,
//	the redactor error messages use, before it can reach an error or
//	the JSON envelope. The stream still reaches the parent's stderr.
package childerr

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/nself-org/cli/internal/output"
)

const (
	MaxLines = 25
	MaxLine  = 400
	keep     = 64 * 1024 // bytes held while collecting
)

// Tail collects the end of a child's stderr.
type Tail struct {
	mu  sync.Mutex
	buf []byte
}

// Write keeps the last keep bytes. It never fails, so the child is never
// slowed or broken by the capture.
func (t *Tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > keep {
		t.buf = append([]byte(nil), t.buf[len(t.buf)-keep:]...)
	}
	return len(p), nil
}

// String is the redacted tail: the last non-empty lines joined with " | ".
func (t *Tail) String() string {
	t.mu.Lock()
	raw := string(t.buf)
	t.mu.Unlock()
	var lines []string
	for _, l := range strings.Split(raw, "\n") {
		l = strings.TrimSpace(strings.TrimRight(l, "\r"))
		if l == "" {
			continue
		}
		if len(l) > MaxLine {
			l = l[:MaxLine] + "..."
		}
		lines = append(lines, output.RedactText(l))
	}
	if len(lines) > MaxLines {
		lines = lines[len(lines)-MaxLines:]
	}
	return strings.Join(lines, " | ")
}

// Failure is a child failure that carries the child's stderr tail.
type Failure struct {
	Err  error
	Tail string
}

func (f *Failure) Error() string {
	if f.Tail == "" {
		return f.Err.Error()
	}
	return f.Err.Error() + "; child stderr: " + f.Tail
}

func (f *Failure) Unwrap() error { return f.Err }

// Run runs c with its stderr also kept in a Tail (the parent's stderr when c
// has none) and returns a *Failure when it fails.
func Run(c *exec.Cmd) error {
	var out io.Writer = os.Stderr
	if c.Stderr != nil {
		out = c.Stderr
	}
	tail := &Tail{}
	c.Stderr = io.MultiWriter(out, tail)
	if err := c.Run(); err != nil {
		return &Failure{Err: err, Tail: tail.String()}
	}
	return nil
}
