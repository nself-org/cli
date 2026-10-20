package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

func TestSecretsResolveSourceGuardAudit(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "implicit-project", true: "explicit-project"}[explicit], func(t *testing.T) {
			home, source := t.TempDir(), t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("NSELF_ALLOW_SOURCE_DIR", "")
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
			for _, dir := range []string{"cmd/commands", "internal"} {
				if err := os.MkdirAll(filepath.Join(source, dir), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(source, "go.mod"), []byte("module nself\n\ngo 1.26\n"), 0600); err != nil {
				t.Fatal(err)
			}
			t.Chdir(source)
			root := &cobra.Command{Use: "nself", PersistentPreRunE: RootCmd.PersistentPreRunE, SilenceUsage: true, SilenceErrors: true}
			root.AddCommand(newSecretsResolveCmd())
			args := []string{"resolve", "project/FOO", "--json", "--for-attempt", "source-refusal"}
			if explicit {
				args = append(args, "--project", source)
			}
			root.SetArgs(args)
			root.SetOut(&bytes.Buffer{})
			root.SetErr(&bytes.Buffer{})
			if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "source repository") {
				t.Fatal("expected real source guard refusal", err)
			}
			data, err := os.ReadFile(filepath.Join(home, ".nself", "audit.log"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(data), `"event":"secret-resolve"`) != 1 || !strings.Contains(string(data), `"outcome":"E401"`) {
				t.Fatalf("expected one refusal audit: %s", data)
			}
		})
	}
}

func TestSecretsResolvePreRunSuccessfulExit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cmd := newSecretsResolveCmd()
	exit := errs.Exit(0)
	if got := auditResolvePreRunError(cmd, nil, exit); got != exit {
		t.Fatal("successful exit changed")
	}
	if _, err := os.Stat(filepath.Join(home, ".nself", "audit.log")); !os.IsNotExist(err) {
		t.Fatal("successful pre-run exit audited as refusal", err)
	}
}
