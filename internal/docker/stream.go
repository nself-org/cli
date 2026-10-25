package docker

// stream.go — long-running docker output streams (`logs -f`), D-0711.
//
// Purpose: the one place a command streams a never-ending docker process to the
// operator. Compose.Run discards its output when stdout is not a TTY, so it
// cannot carry `deploy --follow | tee`; Stream forwards stdout and stderr always.
// Inputs: the docker arguments (after "docker"), a working directory and the
// writers (nil discards that stream; callers pass os.Stdout / os.Stderr).
// Outputs: the child's output on the writers. A stop by the caller's context,
// SIGINT or SIGTERM ends the stream with a nil error; a failure to start or a
// non-zero exit is returned.
// Constraints: the child runs in its own process group and a stop sends it
// SIGTERM through that group (SIGKILL after streamWaitDelay), so Ctrl-C leaves
// no orphaned docker or compose plugin behind.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

// streamWaitDelay is how long a stopped stream gets to exit on SIGTERM before
// the child is killed. A var so tests can shorten it.
var streamWaitDelay = 5 * time.Second

// Stream runs `docker args...` in dir until it exits or is stopped, forwarding
// its stdout and stderr to the writers.
func Stream(ctx context.Context, dir string, args []string, stdout, stderr io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	cmd := exec.CommandContext(sigCtx, "docker", args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = stdout, stderr
	setProcGroupAttr(cmd)
	cmd.Cancel = func() error { return termProcessGroup(cmd) }
	cmd.WaitDelay = streamWaitDelay
	err := cmd.Run()
	if sigCtx.Err() != nil {
		killProcessGroup(cmd) // a grandchild that ignored SIGTERM
		return nil
	}
	if err != nil && !errors.Is(err, exec.ErrWaitDelay) {
		return fmt.Errorf("docker %s: %w", firstArg(args), err)
	}
	return nil
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// StreamLines is Stream with stdout delivered line by line to fn. mergeStderr
// sends stderr through fn too; otherwise it goes to stderr (nil discards it). maxLine caps one line: a longer line ends the stream with an
// error instead of a silent stop. An error from fn stops docker and is returned.
func StreamLines(ctx context.Context, dir string, args []string, stderr io.Writer, mergeStderr bool, maxLine int, fn func(line string) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	pr, pw := io.Pipe()
	errw := stderr
	if mergeStderr {
		errw = pw
	}
	done := make(chan error, 1)
	go func() {
		err := Stream(ctx, dir, args, pw, errw)
		_ = pw.Close()
		done <- err
	}()

	var ferr error
	sc := bufio.NewScanner(pr)
	sc.Buffer(make([]byte, 0, min(64*1024, maxLine)), maxLine) // the cap bounds a token too
	for sc.Scan() {
		if ferr = fn(sc.Text()); ferr != nil {
			break
		}
	}
	if ferr == nil {
		if serr := sc.Err(); serr != nil {
			ferr = fmt.Errorf("reading docker output: %w", serr)
		}
	}
	if ferr != nil {
		cancel() // stop docker so it cannot block on a pipe nobody reads
	}
	_ = pr.Close()
	serr := <-done
	if ferr != nil {
		return ferr
	}
	return serr
}
