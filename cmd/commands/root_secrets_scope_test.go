package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestSecretsRepoScope preserves the caller's cwd through the real root hook.
// Inputs: modern and legacy command paths in a monorepo with backend/.env.
// Outputs: cwd assertions before any secrets operation can read or write.
// Constraints: no Docker, network, decryption or command RunE side effects.
func TestSecretsRepoScope(t *testing.T) {
	root := t.TempDir()
	backend := filepath.Join(root, "backend")
	if err := os.Mkdir(backend, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backend, ".env"), []byte("PROJECT_NAME=test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	for _, path := range []string{
		"config secrets resolve", "secrets resolve",
		"config secrets recipients", "secrets recipients",
		"config secrets recipients add", "secrets recipients add",
		"config secrets recipients remove", "secrets recipients remove",
		"config secrets recipients list", "secrets recipients list",
		"config secrets set", "start", "stop", "status", "plugin resolve", "plugin recipients list",
	} {
		for _, project := range []string{"", ".", "relative-project", root} {
			t.Run(path+"/project="+project, func(t *testing.T) {
				t.Chdir(root)
				cmd := &cobra.Command{Use: "nself"}
				for _, name := range strings.Fields(path) {
					child := &cobra.Command{Use: name}
					cmd.AddCommand(child)
					cmd = child
				}
				cmd.Flags().String("project", project, "")
				if err := RootCmd.PersistentPreRunE(cmd, nil); err != nil {
					t.Fatal(err)
				}
				want := backend
				if strings.HasSuffix(path, "secrets resolve") || strings.Contains(path, "secrets recipients") {
					want = root
				}
				got, err := os.Getwd()
				if err != nil {
					t.Fatal(err)
				}
				// EvalSymlinks accounts for macOS temporary-directory aliases.
				want, err = filepath.EvalSymlinks(want)
				if err != nil {
					t.Fatal(err)
				}
				if got != want {
					t.Errorf("cwd = %s, want %s", got, want)
				}
			})
		}
	}
}
