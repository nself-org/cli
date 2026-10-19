package verify

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "init.defaultBranch=main", "-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// gitRepo returns a work tree whose origin (a local bare repo) holds one
// commit on main with the given trust file content.
func gitRepo(t *testing.T, keys string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses a local git remote")
	}
	root := t.TempDir()
	origin, work := filepath.Join(root, "origin.git"), filepath.Join(root, "work")
	gitIn(t, root, "init", "--bare", origin)
	gitIn(t, root, "init", work)
	if err := os.MkdirAll(filepath.Join(work, ".nself", "trust"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, ".nself", "trust", keysName), []byte(keys), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, work, "add", ".")
	gitIn(t, work, "commit", "-m", "trust")
	gitIn(t, work, "remote", "add", "origin", origin)
	gitIn(t, work, "push", "origin", "HEAD:refs/heads/main")
	return work
}

func TestGitCLIShowsCommittedTrustFiles(t *testing.T) {
	work := gitRepo(t, "# committed\n")
	g := GitCLI{Root: work}
	ctx := context.Background()
	if !g.HasOrigin(ctx) {
		t.Fatal("HasOrigin = false for a repo with origin")
	}
	if err := g.Fetch(ctx); err != nil {
		t.Fatal(err)
	}
	ref, err := g.DefaultRef(ctx)
	if err != nil || ref != "origin/main" {
		t.Fatalf("DefaultRef = %q, %v", ref, err)
	}
	data, found, err := g.Show(ctx, ref, trustRel+keysName)
	if err != nil || !found || string(data) != "# committed\n" {
		t.Fatalf("Show = %q found=%v err=%v", data, found, err)
	}
	// A working-tree edit never changes what Show returns.
	if err := os.WriteFile(filepath.Join(work, ".nself", "trust", keysName), []byte("# edited\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if data, _, _ := g.Show(ctx, ref, trustRel+keysName); string(data) != "# committed\n" {
		t.Fatalf("Show followed the working tree: %q", data)
	}
	if data, found, err := g.Show(ctx, ref, trustRel+revokedName); err != nil || found || data != nil {
		t.Fatalf("absent file: %q found=%v err=%v", data, found, err)
	}
	// A project in a subdirectory of the repository reads its own .nself.
	sub := filepath.Join(work, "app")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := (GitCLI{Root: sub}).Show(ctx, ref, trustRel+keysName); found {
		t.Fatal("subdirectory root resolved a path relative to the repository top")
	}
	if err := os.MkdirAll(filepath.Join(sub, ".nself", "trust"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, ".nself", "trust", keysName), []byte("# sub\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, work, "add", "app")
	gitIn(t, work, "commit", "-m", "sub trust")
	gitIn(t, work, "push", "origin", "HEAD:refs/heads/main")
	if err := g.Fetch(ctx); err != nil {
		t.Fatal(err)
	}
	if data, found, err := (GitCLI{Root: sub}).Show(ctx, ref, trustRel+keysName); err != nil || !found || string(data) != "# sub\n" {
		t.Fatalf("subdirectory project: %q found=%v err=%v", data, found, err)
	}
}

func TestGitCLIRefusesHostileInput(t *testing.T) {
	g := GitCLI{Root: t.TempDir()}
	for _, ref := range []string{"main", "--output=x", "origin/", "origin/-x", "origin/a b", ""} {
		if _, _, err := g.Show(context.Background(), ref, "x"); err == nil || !strings.Contains(err.Error(), "refusing") {
			t.Errorf("ref %q: err = %v, want refusal", ref, err)
		}
	}
	if _, _, err := g.Show(context.Background(), "origin/main", "-x"); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Errorf("dash path: err = %v, want refusal", err)
	}
}

func TestGitCLINoOriginAndBrokenFetch(t *testing.T) {
	work := gitRepo(t, "# k\n")
	gitIn(t, work, "remote", "set-url", "origin", filepath.Join(work, "missing.git"))
	g := GitCLI{Root: work}
	if err := g.Fetch(context.Background()); err == nil {
		t.Fatal("Fetch from a missing remote succeeded")
	}
	gitIn(t, work, "remote", "remove", "origin")
	if g.HasOrigin(context.Background()) {
		t.Fatal("HasOrigin = true without a remote")
	}
	if _, err := g.DefaultRef(context.Background()); err == nil {
		t.Fatal("DefaultRef succeeded without origin refs")
	}
}

func TestSymrefBranch(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"ref: refs/heads/main\tHEAD\n0123\tHEAD\n", "main", true},
		{"ref: refs/heads/feat/x\tHEAD\n", "feat/x", true},
		{"0123\tHEAD\n", "", false},
		{"ref: refs/heads/\tHEAD\n", "", false},
		{"ref: refs/heads/main\n", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := symrefBranch(tc.in)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("symrefBranch(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.wantOK)
		}
	}
}

// TestGitCLIDefaultRefFollowsTheRemote proves the remote's HEAD wins over a
// stale local origin/HEAD.
func TestGitCLIDefaultRefFollowsTheRemote(t *testing.T) {
	work := gitRepo(t, "# k\n")
	g := GitCLI{Root: work}
	ctx := context.Background()
	if err := g.Fetch(ctx); err != nil {
		t.Fatal(err)
	}
	// Point the local origin/HEAD at a branch the remote does not default to.
	gitIn(t, work, "push", "origin", "HEAD:refs/heads/other")
	gitIn(t, work, "fetch", "origin")
	gitIn(t, work, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/other")
	ref, err := g.DefaultRef(ctx)
	if err != nil || ref != "origin/main" {
		t.Fatalf("DefaultRef = %q, %v; want origin/main from the remote", ref, err)
	}
}
