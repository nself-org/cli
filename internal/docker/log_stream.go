package docker

// Purpose: stream Docker logs through the process funnel, including to pipes.
// Inputs: context, working directory, complete docker argv and output writers.
// Outputs: live logs or the child/consumer error; cancellation ends cleanly.
// Constraints: SIGINT/SIGTERM cancel the entire child process group. Writers
// are always used regardless of terminal detection. No container is stopped.

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

// StreamLogs runs complete Docker args and forwards both streams unconditionally.
// Nil writers discard output. Cancellation is a normal end to a log stream.
func (c *Compose) StreamLogs(parent context.Context, workdir string, args []string, stdout, stderr io.Writer) error {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	cmd := exec.CommandContext(ctx, c.dockerBin(), args...)
	cmd.Dir = workdir
	cmd.Stdout, cmd.Stderr = orDiscard(stdout), orDiscard(stderr)
	cmd.WaitDelay = 5 * time.Second
	setProcGroupAttr(cmd)
	cmd.Cancel = func() error {
		killProcessGroup(cmd)
		return cmd.Process.Kill()
	}
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil
	}
	return err
}

// StreamLogLines delivers stdout lines to consume; nil stderr merges stderr
// into the same stream. A consumer/read failure cancels Docker and is returned.
// Lines are capped at 4 MiB, matching the CLI's structured log stream limit.
func (c *Compose) StreamLogLines(parent context.Context, workdir string, args []string, stderr io.Writer, consume func(string) error) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	r, w := io.Pipe()
	defer r.Close()
	if stderr == nil {
		stderr = w
	}
	done := make(chan error, 1)
	go func() {
		err := c.StreamLogs(ctx, workdir, args, w, stderr)
		_ = w.CloseWithError(err)
		done <- err
	}()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
	var consumeErr error
	for scanner.Scan() {
		if consumeErr = consume(scanner.Text()); consumeErr != nil {
			break
		}
	}
	if consumeErr == nil && scanner.Err() != nil {
		consumeErr = fmt.Errorf("reading docker output: %w", scanner.Err())
	}
	// Close before waiting: a failed consumer must unblock the writer goroutine.
	cancel()
	_ = r.Close()
	runErr := <-done
	if consumeErr != nil {
		return consumeErr
	}
	return runErr
}
