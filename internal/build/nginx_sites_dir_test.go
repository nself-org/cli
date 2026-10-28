package build

// nginx_sites_dir_test.go — cli#385: proves resolveNginxSitesDir targets
// the fronting stack's own nginx/sites/ when NGINX_FRONTED_BY's structural
// convention can be confirmed, leaves the unfronted default untouched, and
// refuses (naming both directories) when the convention cannot be
// confirmed rather than silently writing a tree nginx never reads.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// TestResolveNginxSitesDir_Unfronted proves the default (FrontedBy unset)
// case is unchanged: the project's own workdir/nginx/sites.
func TestResolveNginxSitesDir_Unfronted(t *testing.T) {
	workdir := t.TempDir()

	got, err := resolveNginxSitesDir(workdir, "")
	if err != nil {
		t.Fatalf("resolveNginxSitesDir: %v", err)
	}
	want := filepath.Join(workdir, "nginx", "sites")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestResolveNginxSitesDir_FrontedResolved proves that when this project
// lives as "backend" directly under the fronting stack's own directory
// (basename == FrontedBy), route confs are targeted at the fronting
// stack's own nginx/sites — the directory its nginx actually serves from —
// not this project's own (unread) tree.
func TestResolveNginxSitesDir_FrontedResolved(t *testing.T) {
	base := t.TempDir()
	frontingDir := filepath.Join(base, "nself-web")
	workdir := filepath.Join(frontingDir, "backend")
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		t.Fatalf("mkdir workdir: %v", err)
	}

	got, err := resolveNginxSitesDir(workdir, "nself-web")
	if err != nil {
		t.Fatalf("resolveNginxSitesDir: %v", err)
	}
	want := filepath.Join(frontingDir, "nginx", "sites")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// Must NOT be this project's own (unserved) directory.
	if got == filepath.Join(workdir, "nginx", "sites") {
		t.Error("resolved to this project's own nginx/sites — that tree is never read by any running nginx when fronted")
	}
}

// TestResolveNginxSitesDir_FrontedUnresolvedRefuses is the refusal path:
// FrontedBy is set but the parent directory's basename does not match it,
// so the fronting stack's directory cannot be confirmed. The build must
// refuse, and the error must name BOTH this project's own directory and
// the (mismatched) parent directory that was actually checked, so an
// operator can act on the message without re-deriving the topology.
func TestResolveNginxSitesDir_FrontedUnresolvedRefuses(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "some-other-name")
	workdir := filepath.Join(parent, "backend")
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		t.Fatalf("mkdir workdir: %v", err)
	}

	_, err := resolveNginxSitesDir(workdir, "nself-web")
	if err == nil {
		t.Fatal("expected an error when the fronting stack's directory cannot be confirmed, got nil")
	}

	ownDir := filepath.Join(workdir, "nginx", "sites")
	msg := err.Error()
	if !strings.Contains(msg, ownDir) {
		t.Errorf("error must name this project's own directory %q, got: %s", ownDir, msg)
	}
	if !strings.Contains(msg, parent) {
		t.Errorf("error must name the checked (mismatched) parent directory %q, got: %s", parent, msg)
	}
	if !strings.Contains(msg, "nself-web") {
		t.Errorf("error must name the NGINX_FRONTED_BY stack, got: %s", msg)
	}
}

// TestResolveNginxSitesDir_FrontedAtFilesystemRoot guards the edge case
// where workdir has no parent (filepath.Dir returns itself) — must refuse,
// never loop or panic.
func TestResolveNginxSitesDir_FrontedAtFilesystemRoot(t *testing.T) {
	root := string(filepath.Separator)
	if _, err := resolveNginxSitesDir(root, "nself-web"); err == nil {
		t.Error("expected an error resolving a fronting dir at filesystem root, got nil")
	}
}

// TestResolveNginxSitesDir_SymlinkOutsideProject (D-0757): a project's own nginx/sites that is a
// symlink to a directory outside the project is refused in v1.5, because the plan lists in-tree
// paths while the write lands at the link target. v1.4 keeps writing through it.
func TestResolveNginxSitesDir_SymlinkOutsideProject(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	linked := func(t *testing.T, viaParent bool) string {
		workdir, outside := t.TempDir(), t.TempDir()
		nginx := filepath.Join(workdir, "nginx")
		if err := os.MkdirAll(nginx, 0o755); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(nginx, "sites")
		if viaParent {
			// nginx itself is the link and sites does not exist yet
			if err := os.RemoveAll(nginx); err != nil {
				t.Fatal(err)
			}
			target = nginx
		}
		if err := os.Symlink(outside, target); err != nil {
			t.Fatal(err)
		}
		return workdir
	}
	for _, viaParent := range []bool{false, true} {
		workdir := linked(t, viaParent)
		compattest.Set(t, true)
		if _, err := resolveNginxSitesDir(workdir, ""); err == nil || !strings.Contains(err.Error(), "outside the project") {
			t.Errorf("viaParent=%v: v1.5 must refuse, got %v", viaParent, err)
		}
		compattest.Set(t, false)
		if _, err := resolveNginxSitesDir(workdir, ""); err != nil {
			t.Errorf("viaParent=%v: v1.4 keeps the old behaviour, got %v", viaParent, err)
		}
	}
	// A symlink that stays inside the project, and a real or missing directory, are fine.
	compattest.Set(t, true)
	workdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, "shared-sites"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workdir, "nginx"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(workdir, "shared-sites"), filepath.Join(workdir, "nginx", "sites")); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveNginxSitesDir(workdir, ""); err != nil {
		t.Errorf("a link that stays in the project: %v", err)
	}
	if _, err := resolveNginxSitesDir(t.TempDir(), ""); err != nil {
		t.Errorf("a project with no nginx directory yet: %v", err)
	}
}
