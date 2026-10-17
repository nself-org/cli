package commands

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

// A production restore must require the typed project name even with --yes,
// before resolving the backup or starting any database command.
func TestBackupRestoreProductionYesRequiresProjectName(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, ".env"), []byte("PROJECT_NAME=restore-confirm\nENV=prod\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(project)
	t.Setenv("PROJECT_NAME", "restore-confirm")
	t.Setenv("ENV", "prod")
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.Flags().Bool("yes", true, "")
	cmd.SetIn(strings.NewReader("wrong-project\n"))
	var out bytes.Buffer
	cmd.SetErr(&out)
	err := runBackupRestore(cmd, []string{"missing.dump"})
	if err == nil || !strings.Contains(err.Error(), "confirmation") {
		t.Fatalf("production --yes must reject wrong project before restore, got %v", err)
	}
}

func TestBackupRestoreConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name, env, project, input string
		yes, wantOK               bool
	}{
		{"local yes", "dev", "project", "", true, true},
		{"local typed yes", "dev", "project", "yes\n", false, true},
		{"local refusal", "dev", "project", "no\n", false, false},
		{"local EOF", "dev", "project", "", false, false},
		{"prod typed name", "prod", "project", "project\n", true, true},
		{"production typed name", "production", "project", "project\n", false, true},
		{"prod yes is insufficient", "prod", "project", "yes\n", true, false},
		{"prod EOF", "prod", "project", "", true, false},
		{"prod empty name", "prod", "", "\n", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.SetIn(strings.NewReader(tc.input))
			var out bytes.Buffer
			cmd.SetErr(&out)
			err := confirmBackupRestore(cmd, &config.Config{Env: tc.env, ProjectName: tc.project}, tc.yes)
			if (err == nil) != tc.wantOK {
				t.Fatalf("confirmation error=%v, want success=%v", err, tc.wantOK)
			}
			if tc.env == "dev" && tc.yes && out.Len() != 0 {
				t.Fatalf("local --yes should skip the prompt, got %q", out.String())
			}
		})
	}
}
