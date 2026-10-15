//go:build !windows

package commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLogStreamCancellationKillsComposeChild(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "orphan")
	t.Setenv("LOG_ORPHAN_MARKER", marker)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	script := "#!/bin/sh\n(sleep 1; touch \"$LOG_ORPHAN_MARKER\") &\necho ready\nwait\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	stop := errors.New("consumer stopped")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := streamLogLines(ctx, dir, []string{"compose", "logs", "-f"}, LogsOptions{Follow: true}, func(string) error { return stop })
	if !errors.Is(err, stop) {
		t.Fatalf("got %v, want consumer error", err)
	}
	time.Sleep(1500 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("compose child survived cancellation: marker stat = %v", err)
	}
}
