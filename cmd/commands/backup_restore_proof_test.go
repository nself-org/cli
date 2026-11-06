package commands

// Purpose: pin C2-01 and C2-03 at the command layer: --confirm-project is a
// non-interactive proof for `backup restore` and `backup restore-remote`, a
// wrong name is refused everywhere, and restore-remote follows the same rule
// as restore (bare --yes no longer skips the production confirmation).

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/spf13/cobra"
)

func TestBackupRestoreConfirmProject(t *testing.T) {
	for _, tc := range []struct {
		name, env, project, proof string
		wantOK                    bool
	}{
		{"prod matching name", "prod", "project", "project", true},
		{"production matching name", "production", "project", "project", true},
		{"prod wrong name", "prod", "project", "other", false},
		{"prod empty project", "prod", "", "x", false},
		{"local matching name", "dev", "project", "project", true},
		{"local wrong name", "dev", "project", "other", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.SetIn(strings.NewReader("")) // EOF: a remote run has no stdin
			var out bytes.Buffer
			cmd.SetErr(&out)
			err := confirmBackupRestoreWith(cmd, &config.Config{Env: tc.env, ProjectName: tc.project}, true, tc.proof)
			if (err == nil) != tc.wantOK {
				t.Fatalf("error=%v, want success=%v", err, tc.wantOK)
			}
			if out.Len() != 0 {
				t.Fatalf("--confirm-project must not prompt, got %q", out.String())
			}
		})
	}
}

func prodRestoreProject(t *testing.T) {
	t.Helper()
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, ".env"), []byte("PROJECT_NAME=restore-confirm\nENV=prod\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(project)
	t.Setenv("PROJECT_NAME", "restore-confirm")
	t.Setenv("ENV", "prod")
}

// Over ssh (no stdin) the proof flag gets past the confirmation; the restore
// then fails on the missing backup, not on confirmation.
func TestBackupRestoreProductionConfirmProjectNoStdin(t *testing.T) {
	prodRestoreProject(t)
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.Flags().Bool("yes", true, "")
	cmd.Flags().String("confirm-project", "restore-confirm", "")
	cmd.SetIn(strings.NewReader(""))
	cmd.SetErr(&bytes.Buffer{})
	err := runBackupRestore(cmd, []string{"missing.dump"})
	if err == nil || strings.Contains(err.Error(), "confirmation") {
		t.Fatalf("want past confirmation to a missing-backup error, got %v", err)
	}
}

func TestBackupRestoreRemoteProductionYesRequiresProof(t *testing.T) {
	prodRestoreProject(t)
	for name, proof := range map[string]string{"bare --yes": "", "wrong name": "wrong-project"} {
		t.Run(name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			cmd.Flags().String("from", "path:///nonexistent/p_full_1.dump", "")
			cmd.Flags().String("key", "", "")
			cmd.Flags().Bool("yes", true, "")
			cmd.Flags().String("confirm-project", proof, "")
			cmd.SetIn(strings.NewReader(""))
			cmd.SetErr(&bytes.Buffer{})
			err := runBackupRestoreRemote(cmd, nil)
			if err == nil || !strings.Contains(err.Error(), "confirmation") {
				t.Fatalf("production restore-remote must refuse before restoring, got %v", err)
			}
		})
	}
}
