package commands

// Purpose: D-0525. `db drift --metadata --env X` and `db reconcile --env X`
// say so on stderr when a local directory named X shadows the environment X,
// and their --env usage no longer calls the value a project directory.

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestResolveDBMetadataDir_DirShadowingEnvNameWarns(t *testing.T) {
	proj := t.TempDir()
	t.Chdir(proj)
	if err := os.Mkdir("staging", 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := newDBRemoteTestCmd()
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	if err := cmd.Flags().Set("env", "staging"); err != nil {
		t.Fatal(err)
	}
	dir, handled, err := resolveDBMetadataDir(cmd, "db", "reconcile")
	if err != nil || handled || dir != "staging" {
		t.Fatalf("dir=%q handled=%v err=%v, want the directory used", dir, handled, err)
	}
	if !strings.Contains(stderr.String(), "directory here and also an environment name") {
		t.Errorf("no shadow notice on stderr: %q", stderr.String())
	}
}

// Negative control: an absolute path is not an environment name, so no notice.
func TestResolveDBMetadataDir_PlainDirectoryIsQuiet(t *testing.T) {
	t.Chdir(t.TempDir())
	other := t.TempDir()
	cmd := newDBRemoteTestCmd()
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	if err := cmd.Flags().Set("env", other); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolveDBMetadataDir(cmd, "db", "drift", "--metadata"); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Errorf("unexpected notice: %q", stderr.String())
	}
}

func TestDBEnvFlagUsageNamesEnvironment(t *testing.T) {
	for name, f := range map[string]string{
		"db drift":     dbDriftCmd.Flags().Lookup("env").Usage,
		"db reconcile": dbReconcileCmd.Flags().Lookup("env").Usage,
	} {
		if strings.HasPrefix(f, "Project directory") || !strings.Contains(f, "Environment name") {
			t.Errorf("%s --env usage = %q", name, f)
		}
	}
}
