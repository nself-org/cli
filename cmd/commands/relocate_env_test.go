package commands

// Purpose: D-0905 regression. The first `nself ci` of a fresh v1.5 project
// auto-adds the ci plugin; the installer loads the project config, which
// exports the whole .env cascade into the process. The plugin must still see
// the environment a mounted plugin sees: no project secrets, and the shell's
// own values where .env overrides them.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// envDumpScript writes the whole environment of the plugin to $ENV_PROOF.
const envDumpScript = "#!/bin/sh\nenv > \"$ENV_PROOF\"\n"

func TestRelocateAutoAddKeepsProjectEnvOut(t *testing.T) {
	for _, tc := range []struct {
		name        string
		shellValue  string // value exported in the shell for SHELL_SET ("" = unset)
		wantAbsent  []string
		wantPresent map[string]string
	}{
		{"cascade secrets stay out", "", []string{"POSTGRES_PASSWORD", "HASURA_GRAPHQL_ADMIN_SECRET", "PROJECT_ONLY"}, nil},
		{"shell value wins over .env", "from-shell", []string{"POSTGRES_PASSWORD"}, map[string]string{"SHELL_SET": "from-shell"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("the plugin stub is a /bin/sh script")
			}
			proof, _, _ := relocateSetup(t, "", nil)
			// Other tests in the package export the same names into the shared
			// process; start from a clean slate (t.Setenv restores them).
			for _, k := range []string{"POSTGRES_PASSWORD", "HASURA_GRAPHQL_ADMIN_SECRET", "PROJECT_ONLY", "SHELL_SET"} {
				t.Setenv(k, "")
				_ = os.Unsetenv(k)
			}
			tb := aiTarball(t, map[string]string{"nself-ci": envDumpScript})
			sum := aiSum(tb)
			aiServe(t, tb, sum, aiSigned(t, sum))
			autoAddPlugin = autoInstallPlugin
			if tc.shellValue != "" {
				t.Setenv("SHELL_SET", tc.shellValue)
			}
			dir, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			write := func(name, body string) {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			write(".env", "PROJECT_ONLY=1\nSHELL_SET=from-dotenv\n")
			write(".env.secrets", "POSTGRES_PASSWORD=hunter2\nHASURA_GRAPHQL_ADMIN_SECRET=admin-secret\n")

			handled, err := relocateBreakout("ci", nil)
			if !handled || err != nil {
				t.Fatalf("handled=%v err=%v", handled, err)
			}
			data, err := os.ReadFile(proof)
			if err != nil {
				t.Fatalf("the plugin did not run: %v", err)
			}
			got := map[string]string{}
			for _, l := range strings.Split(string(data), "\n") {
				k, v, _ := strings.Cut(l, "=")
				got[k] = v
			}
			for _, k := range tc.wantAbsent {
				if v, ok := got[k]; ok {
					t.Errorf("the auto-added plugin received %s=%q from the project cascade", k, v)
				}
			}
			for k, want := range tc.wantPresent {
				if got[k] != want {
					t.Errorf("%s = %q in the plugin, want %q", k, got[k], want)
				}
			}
			if _, leaked := os.LookupEnv("POSTGRES_PASSWORD"); leaked {
				t.Error("the process environment kept the cascade after the auto-add")
			}
		})
	}
}
