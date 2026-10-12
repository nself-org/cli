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

// SIGTERM and SIGHUP (terminal closed) must cancel the export context so the exporter's deferred cleanup
// can remove the unscrubbed stage directory.
func TestDBExportSignalCancelsContext(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGHUP} {
		t.Run(sig.String(), func(t *testing.T) { signalCancelsExport(t, sig) })
	}
}

func signalCancelsExport(t *testing.T, sig syscall.Signal) {
	dbExportFixture(t)
	cancelled := false
	exportProject = func(ctx context.Context, _ *config.Config, _ exporter.Options) (exporter.Summary, error) {
		if err := syscall.Kill(os.Getpid(), sig); err != nil {
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
