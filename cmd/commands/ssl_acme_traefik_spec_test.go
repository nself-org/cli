package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAcmeTraefikSpecRefusesMissingSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX symlinks")
	}
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	source, err := json.Marshal(filepath.Join(root, "escape", "missing", "acme.json"))
	if err != nil {
		t.Fatal(err)
	}
	raw := `{"services":{"traefik-config":{"image":"nself/r:1","command":["x"],"volumes":[{"type":"bind","source":` + string(source) + `,"target":"/dynamic"}]}}}`
	if _, err := traefikConfigSpec([]byte(raw), []string{root}); err == nil {
		t.Fatal("accepted a missing mount source below a symlink outside the project")
	}
}

func TestAcmeTraefikMountSourceResolution(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX symlinks")
	}
	root, outside := t.TempDir(), t.TempDir()
	for name, target := range map[string]string{
		"escape": outside, "inside": root, "broken": filepath.Join(outside, "absent"),
		"loop": filepath.Join(root, "loop"), "alias": root,
	} {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, source, allowed string
		want                  bool
	}{
		{"missing in project", "new/acme.json", "", true},
		{"missing allowed root", "new/acme.json", "new", true},
		{"missing through internal symlink", "inside/new/acme.json", "", true},
		{"missing through escaping symlink", "escape/new/acme.json", "", false},
		{"dangling symlink", "broken", "", false},
		{"dangling ancestor", "broken/new/acme.json", "", false},
		{"symlink loop", "loop/new/acme.json", "", false},
		{"file ancestor", "file/new/acme.json", "", false},
		{"symlinked allowed root", "new/acme.json", "alias", true},
		{"dangling allowed root", "broken/new/acme.json", "broken", false},
		{"root itself", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := under(filepath.Join(root, tc.source), []string{filepath.Join(root, tc.allowed)}); got != tc.want {
				t.Errorf("under() = %v, want %v", got, tc.want)
			}
		})
	}
}
