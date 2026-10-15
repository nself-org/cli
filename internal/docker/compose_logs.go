package docker

// Purpose: shared compose logs streaming and raw logs/ps output for CLI reports.
// Inputs: context, project directory, compose arguments and explicit I/O sinks.
// Outputs: bounded lines via callback or unparsed subprocess stdout.
// Constraints: preserve compose argv and callback errors; reap cancelled streams.

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"
)

const maxComposeLogLineBytes = 4 << 20

// logCommand builds a cancellable compose subprocess with configured manifests
// and env files. Killing its process group also stops children holding pipes.
func (c *Compose) logCommand(ctx context.Context, workdir, verb string, args []string) *exec.Cmd {
	argv := append(c.buildBaseArgs(), verb)
	cmd := exec.CommandContext(ctx, c.dockerBin(), append(argv, args...)...)
	cmd.Dir = workdir
	cmd.WaitDelay = 5 * time.Second
	setProcGroupAttr(cmd)
	cmd.Cancel = func() error {
		killProcessGroup(cmd)
		return cmd.Process.Kill()
	}
	return cmd
}

// PsOutput passes compose ps stdout through unchanged, including non-TTY
// writers. args selects the default table, --services, -q or another ps view.
func (c *Compose) PsOutput(ctx context.Context, workdir string, stdout, stderr io.Writer, args ...string) error {
	cmd := c.logCommand(ctx, workdir, "ps", args)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	return cmd.Run()
}

// LogsOutput captures raw compose logs for summary/volume reports. It keeps
// partial-line bytes and exit errors rather than applying the streaming limit.
func (c *Compose) LogsOutput(ctx context.Context, workdir string, args ...string) ([]byte, error) {
	return c.logCommand(ctx, workdir, "logs", args).Output()
}

// StreamLogs calls fn for each compose logs line (without its newline).
// stdin can pass terminal input through in follow mode; nil disconnects input.
// A callback/scanner error cancels the child and wins over the resulting exit
// error. Parent cancellation ends the stream cleanly, matching CLI follow mode.
func (c *Compose) StreamLogs(parent context.Context, workdir string, args []string, stdin io.Reader, stderr io.Writer, fn func(string) error) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	cmd := c.logCommand(ctx, workdir, "logs", args)
	cmd.Stdin, cmd.Stderr = stdin, stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting docker: %w", err)
	}
	readErr := scanComposeLogs(stdout, fn)
	if readErr != nil {
		cancel()
	}
	waitErr := cmd.Wait()
	if readErr != nil {
		return readErr
	}
	if parent.Err() != nil {
		return nil
	}
	return waitErr
}

// scanComposeLogs bounds memory per line and preserves callback failures.
func scanComposeLogs(stdout io.Reader, fn func(string) error) error {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), maxComposeLogLineBytes)
	for scanner.Scan() {
		if err := fn(scanner.Text()); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading docker output: %w", err)
	}
	return nil
}
