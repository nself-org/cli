package commands

// Purpose: pin the v1.4 deploy and promote rollbacks: a production restore needs
// a typed --confirm-project, the restore goes by backup id, and no caller uses
// the retired promote.Rollback.
// Inputs: a fake `nself` on PATH that records its argv; stubbed backup entries.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/backup"
	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

// fakeNself puts a recording `nself` first on PATH and returns the argv file.
func fakeNself(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	out := filepath.Join(t.TempDir(), "argv")
	script := "#!/bin/sh\necho \"$@\" >> '" + out + "'\n"
	if err := os.WriteFile(filepath.Join(bin, "nself"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return out
}

func v14Cmd(project string) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("confirm-project", project, "")
	return cmd
}

func stubV14Backups(t *testing.T, entries ...backup.BackupEntry) {
	t.Helper()
	old := v14ListBackups
	v14ListBackups = func(string) ([]backup.BackupEntry, error) { return entries, nil }
	t.Cleanup(func() { v14ListBackups = old })
}

func TestV14DeployRollbackNeedsTypedProjectOnProd(t *testing.T) {
	argv := fakeNself(t)
	stubV14Backups(t, backup.BackupEntry{ID: "demo_full_1", Tag: "pre-promote-x", Date: time.Now()})
	err := rollbackV14(v14Cmd(""), t.TempDir(), "prod", "pre-promote-x")
	if err == nil || !strings.Contains(err.Error(), "E403") || errs.ExitCodeFor(err) != 4 {
		t.Fatalf("production without --confirm-project: err=%v", err)
	}
	if _, statErr := os.Stat(argv); statErr == nil {
		t.Fatal("a refused rollback ran nself")
	}
}

func TestV14PromoteRollbackPassesTagAndProject(t *testing.T) {
	argv := fakeNself(t)
	now := time.Now()
	stubV14Backups(t,
		backup.BackupEntry{ID: "old_id", Tag: "pre-promote-x", Date: now.Add(-time.Hour)},
		backup.BackupEntry{ID: "new_id", Tag: "pre-promote-x", Date: now},
		backup.BackupEntry{ID: "other_id", Tag: "other", Date: now.Add(time.Hour)},
	)
	if err := rollbackV14(v14Cmd("demo"), t.TempDir(), "prod", "pre-promote-x"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(argv)
	if err != nil {
		t.Fatal(err)
	}
	if want := "backup restore new_id --yes --confirm-project demo"; strings.TrimSpace(string(got)) != want {
		t.Fatalf("restore argv = %q, want %q", strings.TrimSpace(string(got)), want)
	}
}

func TestV14RollbackNeverCallsRetiredRollback(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("glob: %v (%d files)", err, len(files))
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "promote.Rollback(") {
			t.Errorf("%s calls the retired promote.Rollback", f)
		}
	}
}
