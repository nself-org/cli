package commands

import (
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/compat"
	"github.com/spf13/cobra"
)

func TestAdminBuiltinRegistration(t *testing.T) {
	t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
	found := false
	for _, f := range builtinFamilies {
		if f.slug == "admin" {
			found = true
			if f.build() == nil {
				t.Fatal("nil admin tree")
			}
		}
	}
	if !found {
		t.Fatal("admin family missing")
	}
	d := adminDeps()
	if d.LoadHealthConfig == nil || d.OpenBrowserCmd == nil || d.ResolveEnvFile == nil || d.SetEnvKeyInFile == nil || d.ShouldOpenBrowser == nil {
		t.Fatal("admin helper injection is incomplete")
	}
}

func TestAdminRegistryCanonByMode(t *testing.T) {
	for _, v15 := range []bool{false, true} {
		view, err := canon.Effective(v15)
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{"admin", "admin health", "admin projects list"} {
			got, ok := view.Commands[path]
			if !ok {
				t.Fatalf("v15=%v: missing %s", v15, path)
			}
			want := canon.CanonPlugin
			if !v15 {
				want = ""
				if path == "admin" {
					want = canon.CanonPending
				}
			}
			if got.Canon != want {
				t.Errorf("v15=%v: %s canon=%q, want %q", v15, path, got.Canon, want)
			}
		}
	}
}

func TestAdminGroupByMode(t *testing.T) {
	t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
	for _, tc := range []struct{ mode, group string }{{"", groupAccount}, {"1", groupPlugins}} {
		t.Setenv("NSELF_V15", tc.mode)
		r := &cobra.Command{Use: "nself"}
		mountBuiltin(r, builtinFamilies)
		c, _, err := r.Find([]string{"admin"})
		if err != nil || c.GroupID != tc.group {
			t.Fatalf("mode %q: group %q err %v", tc.mode, c.GroupID, err)
		}
		if compat.V15() != (tc.mode == "1") {
			t.Fatal("mode mismatch")
		}
	}
}
