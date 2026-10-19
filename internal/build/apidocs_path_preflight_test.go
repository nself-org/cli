package build

// Purpose: a bad API_DOCS_PATH fails the build before any file is written (D-1140).
// Inputs:  the dev-minimal plan fixture with a bad API_DOCS_PATH in .env.
// Outputs: none (t.Error on assertion failure).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildBadAPIDocsPathWritesNothing(t *testing.T) {
	f := newPlanFixture(t, "dev-minimal")
	for _, k := range []string{"API_DOCS_ENABLED", "API_DOCS_PATH"} {
		t.Setenv(k, "")
	}
	envPath := filepath.Join(f.workdir, ".env")
	b, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, append(b, []byte("API_DOCS_PATH=/x;\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(envPath, 0o644); err != nil { // WriteFile keeps the old mode
		t.Fatal(err)
	}
	secrets := filepath.Join(f.workdir, ".env.secrets")
	_ = os.Remove(secrets)

	_, err = Build(f.workdir, BuildOptions{Force: true})
	if err == nil || !strings.Contains(err.Error(), "API_DOCS_PATH") {
		t.Fatalf("err = %v, want an API_DOCS_PATH error", err)
	}
	if _, statErr := os.Stat(secrets); !os.IsNotExist(statErr) {
		t.Errorf(".env.secrets was written before API_DOCS_PATH was validated (stat err = %v)", statErr)
	}
	st, err := os.Stat(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Mode().Perm(); got != 0o644 {
		t.Errorf(".env mode = %o, want 644 untouched before validation", got)
	}
}
