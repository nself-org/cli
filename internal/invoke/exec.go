package invoke

// Purpose: run the nself binary as a child with the machine environment.
// Inputs: a Spec. Outputs: a Result (stdout, stderr tail, exit status).
// Constraints: exec.CommandContext with an argv slice, never a shell; no stdin;
// stdout capped at MaxStdoutBytes (the rest is drained so the child never
// blocks), stderr kept as a ring of StderrRingBytes; SIGTERM to the child's
// process group on cancel, then SIGKILL to the group once the child is gone
// or killDelay has passed. The child env is the parent env minus the machine
// token plus the v1.5 switches (EPIC D2) and, for a timed request, the
// remaining deadline (D16).

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// killDelay is how long a child gets after SIGTERM before SIGKILL. A variable so
// tests can shorten it; production keeps the 5 s of EPIC D2.
var killDelay = 5 * time.Second

// SelfExecutable resolves the binary to run: NSELF_MCP_EXEC_OVERRIDE (tests),
// else this process's own executable. It never resolves a bare "nself" on PATH.
func SelfExecutable() (string, error) {
	if override := os.Getenv(SelfExecOverrideEnv); override != "" {
		return override, nil
	}
	return os.Executable()
}

// ownedEnv are the variables the invoker sets itself; an inherited value is dropped.
var ownedEnv = map[string]bool{
	TokenEnv: true, "NSELF_V15": true, "NSELF_NONINTERACTIVE": true, "CI": true,
	"NO_COLOR": true, InvokedByEnv: true, DeadlineEnv: true,
}

// childEnv builds the child environment. deadline <= 0 means no deadline value.
func childEnv(parent []string, transport string, deadline time.Duration) ([]string, error) {
	invokedBy := ""
	switch transport {
	case TransportMCP:
		invokedBy = "mcp"
	case TransportHTTP, TransportHTTPStream:
		invokedBy = "http"
	default:
		return nil, fmt.Errorf("invoke: unknown transport %q", transport)
	}
	env := make([]string, 0, len(parent)+6)
	for _, kv := range parent {
		name, _, found := strings.Cut(kv, "=")
		if found && !ownedEnv[name] {
			env = append(env, kv)
		}
	}
	env = append(env, "NSELF_V15=1", "NSELF_NONINTERACTIVE=1", "CI=1", "NO_COLOR=1", InvokedByEnv+"="+invokedBy)
	if deadline > 0 {
		ms := deadline.Milliseconds()
		if ms < 1 {
			ms = 1
		}
		env = append(env, DeadlineEnv+"="+strconv.FormatInt(ms, 10))
	}
	return env, nil
}

// capBuffer keeps up to max bytes and drops (but accepts) the rest.
type capBuffer struct {
	max  int
	buf  bytes.Buffer
	over bool
}

func (c *capBuffer) Write(p []byte) (int, error) {
	n := len(p)
	room := c.max - c.buf.Len()
	if room < 0 {
		room = 0
	}
	if n > room {
		c.over = true
		p = p[:room]
	}
	c.buf.Write(p)
	return n, nil
}

// ringBuffer keeps the last max bytes written.
type ringBuffer struct {
	max int
	buf []byte
}

func (r *ringBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if n >= r.max {
		r.buf = append(r.buf[:0], p[n-r.max:]...)
		return n, nil
	}
	r.buf = append(r.buf, p...)
	if len(r.buf) > r.max {
		r.buf = append(r.buf[:0], r.buf[len(r.buf)-r.max:]...)
	}
	return n, nil
}

// terminate asks the child's process group to stop; where signals are
// unsupported it kills.
func terminate(p *os.Process) error {
	if err := signalGroup(p, syscall.SIGTERM); err != nil {
		return p.Kill()
	}
	return nil
}

// Exec runs the nself binary with s.Argv. A non-zero exit is not an error: it
// is Result.ExitCode. The error is for a failure to start, and for the parent
// context ending (its error is returned; the child has been stopped).
func Exec(ctx context.Context, s Spec) (Result, error) {
	var res Result
	if len(s.Argv) == 0 {
		return res, errors.New("invoke: empty argv")
	}
	bin, err := SelfExecutable()
	if err != nil {
		return res, fmt.Errorf("invoke: resolving the nself executable: %w", err)
	}
	runCtx, cancel := ctx, context.CancelFunc(func() {})
	var remaining time.Duration
	if s.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, s.Timeout)
	}
	defer cancel()
	if dl, ok := runCtx.Deadline(); ok && s.Timeout > 0 {
		remaining = time.Until(dl)
	}
	env, err := childEnv(os.Environ(), s.Transport, remaining)
	if err != nil {
		return res, err
	}

	cmd := exec.CommandContext(runCtx, bin, s.Argv...)
	cmd.Dir = s.Dir
	cmd.Env = env
	ownGroup(cmd)
	var cancelled atomic.Bool
	cmd.Cancel = func() error { cancelled.Store(true); return terminate(cmd.Process) }
	cmd.WaitDelay = killDelay
	out := &capBuffer{max: MaxStdoutBytes}
	errTail := &ringBuffer{max: StderrRingBytes}
	if s.Stdout != nil {
		cmd.Stdout = s.Stdout
	} else {
		cmd.Stdout = out
	}
	cmd.Stderr = errTail

	waitErr := cmd.Run()
	if cancelled.Load() && cmd.Process != nil {
		killGroup(cmd.Process) // grandchildren that ignored SIGTERM or outlived the child
	}
	res.Stderr = errTail.buf
	if s.Stdout == nil {
		res.Stdout = out.buf.Bytes()
		res.StdoutTruncated = out.over
	}
	if ctx.Err() != nil {
		res.ExitCode = -1
		return res, ctx.Err()
	}
	if s.Timeout > 0 && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		res.TimedOut = true
		res.ExitCode = -1
		return res, nil
	}
	var exitErr *exec.ExitError
	switch {
	case waitErr == nil, errors.Is(waitErr, exec.ErrWaitDelay):
		res.ExitCode = 0
	case errors.As(waitErr, &exitErr):
		res.ExitCode = exitErr.ExitCode()
	default:
		res.ExitCode = -1
		return res, fmt.Errorf("invoke: running the nself child: %w", waitErr)
	}
	return res, nil
}
