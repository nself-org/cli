package commands

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/reconcile"
)

func TestDoctorDriftProjectPluginDir(t *testing.T) {
	isolateDoctorHome(t)
	root, plugins := t.TempDir(), t.TempDir()
	shell := t.TempDir()
	t.Setenv("NSELF_PLUGIN_DIR", shell)
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("PROJECT_NAME=fixture\nENV=dev\nNSELF_PLUGIN_DIR="+plugins+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const key = reconcile.PluginsPrefix + "fragment.yml"
	generated := []byte("generated\n")
	state := &reconcile.GeneratedState{Files: map[string]string{key: fmt.Sprintf("%x", sha256.Sum256(generated))}}
	if err := state.Save(root); err != nil {
		t.Fatal(err)
	}
	for _, edited := range []bool{false, true} {
		body := generated
		if edited {
			body = []byte("hand edited\n")
		}
		if err := os.WriteFile(filepath.Join(plugins, "fragment.yml"), body, 0o644); err != nil {
			t.Fatal(err)
		}
		// An unrelated shell plugin must neither hide nor invent drift.
		if err := os.WriteFile(filepath.Join(shell, "fragment.yml"), []byte("shell edit\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		r, p, err := checkGeneratedDrift(context.Background(), root)
		if err != nil || (r.Status == "fail") != edited {
			t.Fatalf("edited=%v: %+v, %v", edited, r, err)
		}
		if edited && (len(p.Artifacts) != 1 || !p.Artifacts[0].HandEdited) {
			t.Fatalf("missing hand edit: %+v", p)
		}
	}
	legacy := reconcile.AbsPrefix + strings.TrimPrefix(filepath.ToSlash(filepath.Join(plugins, "fragment.yml")), "/")
	state.Files[legacy] = state.Files[key]
	delete(state.Files, key)
	if err := state.Save(root); err != nil {
		t.Fatal(err)
	}
	r, _, err := checkGeneratedDrift(context.Background(), root)
	if err != nil || r.Status != "fail" {
		t.Fatalf("legacy hand edit missed: %+v, %v", r, err)
	}
}
