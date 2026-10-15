package commands

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

// TestLogsRunningServicesUsesProjectRoot ensures nested callers query the
// same compose project used by logs and report samples.
func TestLogsRunningServicesUsesProjectRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "services"), []byte("api\ndb\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	stub := "#!/bin/sh\n[ \"$*\" = 'compose ps --services' ] || exit 9\ncat services\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	got, err := getRunningServices(context.Background(), dir)
	if err != nil || !reflect.DeepEqual(got, []string{"api", "db"}) {
		t.Fatalf("services=%q, err=%v", got, err)
	}
}
