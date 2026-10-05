package docker

// exec_stdin.go — run a command in a running container, feeding it stdin,
// and capture its output.
//
// Purpose: `psql` style calls that pipe SQL in (`docker exec -i`) or only read
// a result (nil stdin), all funnelled through this package (EPIC G1/G4).
// Inputs: a container name, the command with its arguments, an optional stdin.
// Outputs: stdout and stderr as raw strings; the unwrapped run error so the
// caller can word its own message around stderr.
// Constraints: no TTY. `-i` is passed only when stdin is non-nil. ExecHook is
// a test seam that counts every exec issued through ExecStdin.

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"sync"
)

var (
	hookMu   sync.RWMutex
	execHook func(container string, cmd []string)
)

// SetExecHook installs fn to be called once per ExecStdin call (test seam for
// exec counting) and returns a function that restores the previous hook.
func SetExecHook(fn func(container string, cmd []string)) (restore func()) {
	hookMu.Lock()
	prev := execHook
	execHook = fn
	hookMu.Unlock()
	return func() {
		hookMu.Lock()
		execHook = prev
		hookMu.Unlock()
	}
}

// ExecStdin runs `docker exec [-i] <container> <cmd...>` with stdin (nil: no
// stdin, no -i) and returns the captured output. A non-zero exit returns the
// exec error as is; stderr is returned alongside for the caller to quote.
func ExecStdin(ctx context.Context, container string, cmd []string, stdin io.Reader) (stdout, stderr string, err error) {
	hookMu.RLock()
	h := execHook
	hookMu.RUnlock()
	if h != nil {
		h(container, cmd)
	}
	args := []string{"exec"}
	if stdin != nil {
		args = append(args, "-i")
	}
	args = append(args, container)
	args = append(args, cmd...)
	c := exec.CommandContext(ctx, "docker", args...)
	c.Stdin = stdin
	var so, se bytes.Buffer
	c.Stdout, c.Stderr = &so, &se
	err = c.Run()
	return so.String(), se.String(), err
}
