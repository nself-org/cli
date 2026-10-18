package plugin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/nself-org/cli/internal/compat"
)

// maxCaptureBytes caps what CaptureOutput keeps of a plugin's stdout. A
// plugin that prints more is cut at the cap and the run reports an error.
const maxCaptureBytes = 4 << 20

// captureWaitDelay bounds how long a killed plugin's inherited pipes may keep
// CaptureOutput waiting (a script whose child outlives it).
const captureWaitDelay = time.Second

// CaptureOutput runs an installed plugin binary with args and returns what it
// wrote to stdout (P7-CI-53; Constitution §2.2, the plugin exec funnel for
// callers that need the output rather than an inherited terminal).
//
// Purpose: let the composition root run a plugin subcommand without its own
// exec.Command, with the same environment execPluginBinary gives (ExecEnv).
//
// Inputs: ctx bounds the run (the process is killed when it ends), binPath is
// an already validated plugin binary path, args is its full argv tail.
//
// Outputs: stdout bytes (at most 4 MiB), the process exit code, and an error.
// A process that ran and exited non-zero is NOT an error: exit carries the
// code and err is nil. err is set when the process could not start, was
// killed by ctx (wraps ctx.Err()), or printed more than the cap; exit is -1
// then.
//
// Constraints: stdin is empty, stderr is discarded, the environment is
// ExecEnv(os.Environ(), compat.V15).
func CaptureOutput(ctx context.Context, binPath string, args []string) (stdout []byte, exit int, err error) {
	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.Env = ExecEnv(os.Environ(), compat.V15)
	cmd.WaitDelay = captureWaitDelay
	out := &cappedBuffer{limit: maxCaptureBytes}
	cmd.Stdout = out
	runErr := cmd.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return out.buf.Bytes(), -1, fmt.Errorf("running %s: %w", binPath, ctxErr)
	}
	if out.over {
		return out.buf.Bytes(), -1, fmt.Errorf("running %s: output exceeds %d bytes", binPath, maxCaptureBytes)
	}
	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
		return out.buf.Bytes(), 0, nil
	case errors.As(runErr, &exitErr):
		return out.buf.Bytes(), exitErr.ExitCode(), nil
	default:
		return out.buf.Bytes(), -1, fmt.Errorf("running %s: %w", binPath, runErr)
	}
}

// cappedBuffer keeps at most limit bytes and notes when it dropped some.
type cappedBuffer struct {
	buf   bytes.Buffer
	limit int
	over  bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if room := c.limit - c.buf.Len(); n > room {
		c.over = true
		p = p[:room]
	}
	c.buf.Write(p)
	// Always report the full length: a short write would make the exec copy
	// loop fail; ctx or the process exit ends the run.
	return n, nil
}
