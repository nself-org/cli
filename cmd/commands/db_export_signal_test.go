//go:build !windows

package commands

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/exporter"
)

// SIGTERM must cancel the export context so the exporter's deferred cleanup
// can remove the unscrubbed stage directory.
func TestDBExportSignalCancelsContext(t *testing.T) {
	dbExportFixture(t)
	cancelled := false
	exportProject = func(ctx context.Context, _ *config.Config, _ exporter.Options) (exporter.Summary, error) {
		if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		select {
		case <-ctx.Done():
			cancelled = true
			return exporter.Summary{}, ctx.Err()
		case <-time.After(5 * time.Second):
			return exporter.Summary{}, nil
		}
	}
	if err := runDBExport(dbExportCmd, nil); err == nil || !cancelled {
		t.Fatal("signal did not cancel the export context", err)
	}
}
