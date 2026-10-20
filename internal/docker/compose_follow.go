package docker

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"
)

// FollowLogs streams Compose logs even when output is redirected. Interrupts
// cancel the command and its process group through the shared Docker runner.
func (c *Compose) FollowLogs(ctx context.Context, workdir string, stdout, stderr io.Writer) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	args := append(c.buildBaseArgs(), "logs", "-f", "--tail=50")
	return c.run(ctx, workdir, stdout, stderr, args...)
}
