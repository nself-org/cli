package reconcile

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// TestPlanTreatsLegacyPluginsFileAsAbsent: D-1201. Old projects carry a
// 22-byte regular file at .nself/plugins. The plan must treat the plugin
// overlays under it as absent instead of failing with "not a directory".
func TestPlanTreatsLegacyPluginsFileAsAbsent(t *testing.T) {
	f := loadFixture(t, "prod-plugins")
	legacy := filepath.Join(f.project, ".nself", "plugins")
	if err := os.RemoveAll(legacy); err != nil {
		t.Fatal(err)
	}
	writeFile(t, legacy, "nself-alpha\nnself-beta\n", 0o644)
	p := planOf(t, f, nil)
	if p == nil {
		t.Fatal("Compute returned no plan")
	}
	if b, err := os.ReadFile(legacy); err != nil || string(b) != "nself-alpha\nnself-beta\n" {
		t.Fatalf("the plan changed the legacy file: %q, %v", b, err)
	}
}

// TestPlanStillFailsOnPermissionDenied: any stat error other than not-exist
// and the D-1201 legacy case still fails the plan.
func TestPlanStillFailsOnPermissionDenied(t *testing.T) {
	f := loadFixture(t, "prod-plugins")
	orig := statFile
	t.Cleanup(func() { statFile = orig })
	statFile = func(string) (os.FileInfo, error) { return nil, os.ErrPermission }
	_, err := Compute(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("fixture-seed")})
	if err == nil || !errors.Is(err, os.ErrPermission) || !strings.Contains(err.Error(), "for the plan") {
		t.Fatalf("Compute error = %v, want a permission error naming the plan read", err)
	}
}

// TestLegacyPluginsFileOnlyMatchesTheLegacyCase: the exemption needs ENOTDIR,
// a path under <project>/.nself/plugins, and a regular file at that root.
func TestLegacyPluginsFileOnlyMatchesTheLegacyCase(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, ".nself", "plugins")
	child := filepath.Join(root, "ai", "docker-compose.plugin.yml")
	enotdir := &fs.PathError{Op: "stat", Path: child, Err: syscall.ENOTDIR}
	if legacyPluginsFile(dir, child, enotdir) {
		t.Fatal("matched while .nself/plugins does not exist")
	}
	writeFile(t, root, "legacy\n", 0o644)
	if !legacyPluginsFile(dir, child, enotdir) {
		t.Fatalf("did not match the legacy case (err %v)", enotdir)
	}
	if legacyPluginsFile(dir, child, os.ErrPermission) {
		t.Fatal("matched a permission error")
	}
	if legacyPluginsFile(dir, filepath.Join(dir, ".nself", "plugins-other", "x"), enotdir) {
		t.Fatal("matched a sibling of .nself/plugins")
	}
	if legacyPluginsFile(dir, filepath.Join(dir, "other", "x"), enotdir) {
		t.Fatal("matched a path outside .nself/plugins")
	}
}
