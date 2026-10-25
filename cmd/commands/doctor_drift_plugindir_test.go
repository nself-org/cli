package commands

// Purpose: the standalone doctor drift check resolves "@plugins/" record keys
// under the plugin directory the project's env cascade names (D-0644).
// Constraints: temp dirs only; the shell leaves NSELF_PLUGIN_DIR unset.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/reconcile"
)

func TestDoctorDriftPluginDirOnlyInProjectEnv(t *testing.T) {
	isolateDoctorHome(t)
	t.Setenv("NSELF_PLUGIN_DIR", "")
	dir, plugins := t.TempDir(), t.TempDir()
	env := "PROJECT_NAME=fx\nENV=dev\nBASE_DOMAIN=example.test\nSSL_MODE=none\nNSELF_PLUGIN_DIR=" + plugins + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	const key = reconcile.PluginsPrefix + "fxplug/docker-compose.plugin.yml"
	fragment := filepath.Join(plugins, "fxplug", "docker-compose.plugin.yml")
	if err := os.MkdirAll(filepath.Dir(fragment), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fragment, []byte("services: {}\n# hand edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("services: {}\n")) // the bytes the record says were written
	st := &reconcile.GeneratedState{Files: map[string]string{key: hex.EncodeToString(sum[:])}}
	if err := st.Save(dir); err != nil {
		t.Fatal(err)
	}
	r, _, err := checkGeneratedDrift(context.Background(), dir)
	if err != nil || r.Status != "fail" || r.Message != "drift: "+key+" (hand-edited)" {
		t.Fatalf("hand-edited fragment under the .env plugin dir: %+v, %v", r, err)
	}
}
