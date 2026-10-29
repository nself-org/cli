package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/output"
)

// TestConfigExplainMonorepoExecution exercises Cobra's full hook order against
// distinct root/backend values. Raw flags must take effect before any redirect,
// and the redirect notice must never precede the JSON document on stdout.
func TestConfigExplainMonorepoExecution(t *testing.T) {
	for _, tc := range []struct {
		name     string
		flags    []string
		want     string
		redirect bool
	}{
		{"disabled", []string{"--no-monorepo"}, "root.test", false},
		{"default", nil, "backend.test", true},
		{"explicit false", []string{"--no-monorepo=false"}, "backend.test", true},
		{"last value wins", []string{"--no-monorepo", "--no-monorepo=false"}, "backend.test", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
			t.Setenv("ENV", "dev")
			t.Setenv("NSELF_LEGACY_ENV_ORDER", "")
			root := envExplainProject(t, map[string]string{".env": "BASE_DOMAIN=root.test\n"})
			backend := filepath.Join(root, "backend")
			if err := os.Mkdir(backend, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(backend, ".env"), []byte("BASE_DOMAIN=backend.test\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"json", "no-monorepo"} {
				flag := RootCmd.PersistentFlags().Lookup(name)
				value, changed := flag.Value.String(), flag.Changed
				if err := flag.Value.Set("false"); err != nil {
					t.Fatal(err)
				}
				flag.Changed = false
				t.Cleanup(func() { _ = flag.Value.Set(value); flag.Changed = changed })
			}
			oldWriter := pilotWriter
			pilotWriter = func() output.Writer { return output.Writer{Out: os.Stdout, Err: os.Stderr} }
			output.ResetState()
			t.Cleanup(func() { pilotWriter = oldWriter; output.ResetState(); RootCmd.SetArgs(nil) })
			args := append([]string{"config", "explain", "BASE_DOMAIN", "--json", "--reveal"}, tc.flags...)
			RootCmd.SetArgs(args)
			var stdout string
			var runErr error
			stderr := captureStderr(t, func() {
				stdout, runErr = captureStdout(t, RootCmd.Execute)
			})
			if runErr != nil {
				t.Fatal(runErr)
			}
			data := singleDoc(t, stdout)["data"].(map[string]any)
			if got := data["effective"].(map[string]any)["value"]; got != tc.want {
				t.Errorf("effective value = %v, want %s", got, tc.want)
			}
			if got := strings.Contains(stderr, "Detected monorepo layout"); got != tc.redirect {
				t.Errorf("redirect notice = %v, want %v: %s", got, tc.redirect, stderr)
			}
			wantDir := root
			if tc.redirect {
				wantDir = backend
			}
			if cwd, err := os.Getwd(); err != nil || cwd != wantDir {
				t.Errorf("cwd = %q (%v), want %q", cwd, err, wantDir)
			}
		})
	}
}
