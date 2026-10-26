package commands

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRelocateInstalledWithLeadingFlags checks that skipped mounting does not
// turn an installed breakout into an automatic install or an E404 refusal.
func TestRelocateInstalledWithLeadingFlags(t *testing.T) {
	manifest, err := os.ReadFile(filepath.Join("testdata", "mount", "demo", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest = []byte(strings.ReplaceAll(string(manifest), "demo", "ci"))
	manifest = []byte(strings.ReplaceAll(string(manifest), "nself-ci", "nself-ci-installed"))
	for _, noAutoAdd := range []string{"", "1"} {
		for _, command := range []string{"ci", "runner"} {
			t.Run(command+"/no-auto-add="+noAutoAdd, func(t *testing.T) {
				proof, calls, warn := relocateSetup(t, "", func(string, string) (string, error) {
					return "", errors.New("installed plugin must not be installed again")
				})
				t.Setenv(envNoAutoAdd, noAutoAdd)
				dir := t.TempDir()
				t.Setenv("NSELF_PLUGIN_DIR", dir)
				for _, sub := range []string{"ci", "bin"} {
					if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.WriteFile(filepath.Join(dir, "ci", "plugin.json"), manifest, 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "bin", "nself-ci-installed"), []byte(stubScript), 0o755); err != nil {
					t.Fatal(err)
				}
				previousArgs := os.Args
				os.Args = []string{"nself", "--json", command, "verify", "x"}
				t.Cleanup(func() { os.Args = previousArgs })
				mountInstalledPluginsFromDisk(RootCmd)
				if mountedBy(walkNames(RootCmd, []string{"ci"}), "ci") {
					t.Fatal("leading root flags must skip mounting for this regression")
				}
				handled, err := relocateInvocation(os.Args[1:])
				if !handled || err != nil {
					t.Fatalf("handled=%v err=%v", handled, err)
				}
				want := "verify x --json"
				if command == "runner" {
					want = "nodes " + want
				}
				if got := readProof(t, proof)["argv"]; got != want {
					t.Fatalf("argv=%q, want %q", got, want)
				}
				if len(*calls) != 0 || warn.Len() != 0 {
					t.Fatalf("installed plugin was added again: calls=%v warning=%q", *calls, warn.String())
				}
			})
		}
	}
}
