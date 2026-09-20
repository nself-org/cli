package docker

// Purpose: composePsEntry-to-ContainerInfo conversion, ComposeConfig, the low-level Run exec helper, and port-string parsing backing ComposePs in compose.go.
// Inputs: raw `docker compose ps`/`config` output and shell command args.
// Outputs: []ContainerInfo, validated compose config, and executed docker compose commands.
// Constraints: split out of compose.go as a pure move (CLI-R12); no behavior change.

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/term"
)

// composePsEntriesToInfos converts a slice of composePsEntry into ContainerInfo.
func composePsEntriesToInfos(entries []composePsEntry) []ContainerInfo {
	infos := make([]ContainerInfo, 0, len(entries))
	for _, e := range entries {
		infos = append(infos, composePsEntryToInfo(e))
	}
	return infos
}

// composePsEntryToInfo converts a single composePsEntry into ContainerInfo.
func composePsEntryToInfo(e composePsEntry) ContainerInfo {
	return ContainerInfo{
		ID:      e.ID,
		Name:    e.Name,
		Service: e.Service,
		Image:   e.Image,
		State:   e.State,
		Health:  e.Health,
		Ports:   parsePorts(e.Ports),
	}
}

// ComposeConfig runs `docker compose config` in the given workdir to validate
// the compose configuration. Returns nil on success.
func (c *Compose) ComposeConfig(ctx context.Context, workdir string) error {
	args := c.buildBaseArgs()
	args = append(args, "config", "--quiet")
	return c.Run(ctx, workdir, args...)
}

// Run executes a docker command with the given arguments, setting the working
// directory and inheriting the context for cancellation. Output is forwarded to
// os.Stdout/os.Stderr via goroutine-owned pipes rather than direct fd inheritance.
//
// Direct fd inheritance (cmd.Stdout = os.Stdout) passes the test binary's stdout
// file descriptor to docker compose and all its children. The docker daemon, which
// is a separate long-lived process, may hold that fd open via IPC even after the
// docker compose process is killed — causing "*** Test I/O incomplete N s after
// exiting" in the Go test harness. Piped copying breaks the inheritance chain: the
// subprocess gets the write end of a pipe; our goroutine owns the read end and
// copies to os.Stdout. When the subprocess exits (or is killed), it closes the
// write end, the goroutine finishes, and the test binary's stdout fd is never
// held by any docker process.
//
// WaitDelay gives the subprocess up to 5 seconds to drain pending I/O after the
// context is cancelled and the process is killed.
//
// Setpgid places docker compose in its own process group so SIGKILL from the
// Cancel hook propagates to all spawned child processes, not just the top-level
// docker process.
func (c *Compose) Run(ctx context.Context, workdir string, args ...string) error {
	cmd := exec.CommandContext(ctx, c.dockerBin(), args...)
	cmd.Dir = workdir
	cmd.WaitDelay = 5 * time.Second

	// Put docker compose and all its spawned children in a new process group so
	// Cancel can kill the entire group with a single signal.
	// setProcGroupAttr and killProcessGroup are implemented per-OS in
	// compose_unix.go (darwin/linux) and compose_windows.go (windows).
	setProcGroupAttr(cmd)
	cmd.Cancel = func() error {
		killProcessGroup(cmd)
		return cmd.Process.Kill()
	}

	// Use pipe-based I/O forwarding instead of direct fd inheritance. This ensures
	// the test binary's stdout/stderr fds are never held open by docker processes.
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("docker %s: stdout pipe: %w", strings.Join(args, " "), err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("docker %s: stderr pipe: %w", strings.Join(args, " "), err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("docker %s: %w", strings.Join(args, " "), err)
	}

	// In interactive TTY sessions forward output to the real terminal so Docker
	// Compose can render progress correctly. In non-TTY contexts (tests, CI,
	// piped output) discard subprocess output — the test harness captures output
	// via cobra's SetOut/SetErr and does not need raw docker progress lines.
	// Using io.Discard here prevents the goroutines from ever touching os.Stdout
	// after the subprocess exits, which eliminates "Test I/O incomplete" failures
	// caused by goroutines writing to os.Stdout past the test deadline.
	outSink := io.Writer(os.Stdout)
	errSink := io.Writer(os.Stderr)
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		outSink = io.Discard
		errSink = io.Discard
	}

	// Always retain a bounded tail of stderr, even when the sink is Discard.
	// Without this a non-TTY failure (all of CI) reports only "exit status 1"
	// and throws away the line that says what actually went wrong — that is
	// how a "no such service" error stayed undiagnosable through a full
	// release cycle. The tail is capped so a chatty failure cannot grow
	// unboundedly in memory.
	errTail := &tailBuffer{limit: stderrTailLimit}

	done := make(chan struct{}, 2)
	go func() { io.Copy(outSink, stdoutPipe); done <- struct{}{} }()                          //nolint:errcheck
	go func() { io.Copy(io.MultiWriter(errSink, errTail), stderrPipe); done <- struct{}{} }() //nolint:errcheck

	waitErr := cmd.Wait()
	<-done
	<-done

	if waitErr != nil {
		if detail := errTail.String(); detail != "" {
			return fmt.Errorf("docker %s: %w: %s", strings.Join(args, " "), waitErr, detail)
		}
		return fmt.Errorf("docker %s: %w", strings.Join(args, " "), waitErr)
	}
	return nil
}

// stderrTailLimit caps how much stderr is retained for error messages.
const stderrTailLimit = 2048

// tailBuffer keeps only the LAST limit bytes written to it. Docker puts the
// actionable line at the end of a failure, so the tail is the useful part.
type tailBuffer struct {
	buf   []byte
	limit int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	n := len(p)
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.limit {
		t.buf = t.buf[len(t.buf)-t.limit:]
	}
	return n, nil
}

// String returns the retained stderr as a single whitespace-collapsed line so
// it reads cleanly when wrapped into an error.
func (t *tailBuffer) String() string {
	return strings.Join(strings.Fields(string(t.buf)), " ")
}

// parsePorts splits a Docker Compose ports string (e.g. "0.0.0.0:80->80/tcp, 0.0.0.0:443->443/tcp")
// into individual port mappings.
func parsePorts(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ", ")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
