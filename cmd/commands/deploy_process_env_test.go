package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

func TestResolvedDeployEnvProcessOnlyRefusal(t *testing.T) {
	compattest.Set(t, true)
	t.Setenv("POSTGRES_PASSWORD", "shell-secret-sentinel")
	dir := t.TempDir()
	path, cleanup, err := writeResolvedDeployEnv(dir, "staging")
	cleanup()
	if err == nil {
		t.Fatalf("snapshot accepted process-only password: %s", path)
	}
	if !strings.Contains(err.Error(), "POSTGRES_PASSWORD") || !strings.Contains(err.Error(), "env file") || strings.Contains(err.Error(), "shell-secret-sentinel") {
		t.Fatalf("unsafe or unactionable refusal: %v", err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		t.Fatalf("refusal left a snapshot: %v, %v", files, err)
	}
}

func TestResolvedDeployEnvFileWins(t *testing.T) {
	compattest.Set(t, true)
	for _, value := range []string{"file-secret", ""} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("POSTGRES_PASSWORD", "shell-secret-sentinel")
			t.Setenv("UNRELATED_SHELL_SECRET", "unrelated-sentinel")
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("POSTGRES_PASSWORD="+value+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			path, cleanup, err := writeResolvedDeployEnv(dir, "staging")
			defer cleanup()
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "sentinel") {
				t.Fatal("snapshot contains shell credentials")
			}
		})
	}
}
