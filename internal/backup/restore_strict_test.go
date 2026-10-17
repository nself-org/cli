package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
)

// fakeDockerPgRestoreError puts a docker stub on PATH whose pg_restore prints
// an error line and exits 1, as a DROP blocked by a dependency does.
func fakeDockerPgRestoreError(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("docker stub is a shell script")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\ncase \"$*\" in\n*pg_restore*) cat >/dev/null; echo 'pg_restore: error: ERROR:  cannot drop table x' >&2; exit 1;;\n*) exit 0;;\nesac\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// Local restores always fail on a pg_restore error line. Compatibility mode
// cannot weaken this rule because the target swap requires a complete archive.
func TestRestorePgDumpStrictFailsOnErrorLineInV14(t *testing.T) {
	compattest.Set(t, false)
	fakeDockerPgRestoreError(t)
	dump := filepath.Join(t.TempDir(), "p_full_1.dump")
	if err := os.WriteFile(dump, []byte("PGDMP"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restorePgDumpErr(dump, true); err == nil || !errors.Is(err, errs.ErrBackupRestoreFailed) || !strings.Contains(err.Error(), "cannot drop table x") {
		t.Fatalf("strict restore must fail on the error line, got %v", err)
	}
	if err := restorePgDumpErr(dump, false); err == nil || !errors.Is(err, errs.ErrBackupRestoreFailed) || !strings.Contains(err.Error(), "cannot drop table x") {
		t.Fatalf("non-strict local restore must also fail on the error line, got %v", err)
	}
}

func restorePgDumpErr(dump string, strict bool) error {
	return restorePgDump(context.Background(), "c", "u", "d", dump, strict)
}
