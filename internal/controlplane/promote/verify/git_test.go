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
	commit, err := g.PinDefault(ctx)
	if err != nil || !commitRE.MatchString(commit) {
		t.Fatalf("PinDefault = %q, %v", commit, err)
	}
	data, found, err := g.Show(ctx, commit, trustRel+keysName)
	if err != nil || !found || string(data) != "# committed\n" {
		t.Fatalf("Show = %q found=%v err=%v", data, found, err)
	}
	// A working-tree edit never changes what Show returns.
	if err := os.WriteFile(filepath.Join(work, ".nself", "trust", keysName), []byte("# edited\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if data, _, _ := g.Show(ctx, commit, trustRel+keysName); string(data) != "# committed\n" {
		t.Fatalf("Show followed the working tree: %q", data)
	}
	if data, found, err := g.Show(ctx, commit, trustRel+revokedName); err != nil || found || data != nil {
		t.Fatalf("absent file: %q found=%v err=%v", data, found, err)
	}
	// A project in a subdirectory of the repository reads its own .nself.
	sub := filepath.Join(work, "app")
	if err := os.MkdirAll(filepath.Join(sub, ".nself", "trust"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := (GitCLI{Root: sub}).Show(ctx, commit, trustRel+keysName); found {
		t.Fatal("subdirectory root resolved a path relative to the repository top")
	}
	if err := os.WriteFile(filepath.Join(sub, ".nself", "trust", keysName), []byte("# sub\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, work, "add", "app")
	gitIn(t, work, "commit", "-m", "sub trust")
	gitIn(t, work, "push", "origin", "HEAD:refs/heads/main")
	commit, err = g.PinDefault(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if data, found, err := (GitCLI{Root: sub}).Show(ctx, commit, trustRel+keysName); err != nil || !found || string(data) != "# sub\n" {
		t.Fatalf("subdirectory project: %q found=%v err=%v", data, found, err)
	}
}

func TestGitCLIRefusesHostileInput(t *testing.T) {
	g := GitCLI{Root: t.TempDir()}
	for _, commit := range []string{"main", "origin/main", "--output=x", "", "abc123", strings.Repeat("g", 40)} {
		if _, _, err := g.Show(context.Background(), commit, "x"); err == nil || !strings.Contains(err.Error(), "refusing") {
			t.Errorf("commit %q: err = %v, want refusal", commit, err)
		}
	}
	if _, _, err := g.Show(context.Background(), strings.Repeat("a", 40), "-x"); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Errorf("dash path: err = %v, want refusal", err)
	}
}

func TestGitCLINoOriginAndBrokenRemote(t *testing.T) {
	work := gitRepo(t, "# k\n")
	gitIn(t, work, "remote", "set-url", "origin", filepath.Join(work, "missing.git"))
	g := GitCLI{Root: work}
	if _, err := g.PinDefault(context.Background()); err == nil {
		t.Fatal("PinDefault against a missing remote succeeded")
	}
	gitIn(t, work, "remote", "remove", "origin")
	if g.HasOrigin(context.Background()) {
		t.Fatal("HasOrigin = true without a remote")
	}
	if _, err := g.PinDefault(context.Background()); err == nil {
		t.Fatal("PinDefault succeeded without origin")
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

// TestGitCLIPinIgnoresLocalLookalikes: a local tag named like the remote
// branch, a stale origin/HEAD and a narrowed fetch refspec cannot change which
// commit is trusted.
func TestGitCLIPinIgnoresLocalLookalikes(t *testing.T) {
	work := gitRepo(t, "# real\n")
	g := GitCLI{Root: work}
	ctx := context.Background()
	// A hostile local tag with the same name as the tracking ref, carrying a different key file.
	if err := os.WriteFile(filepath.Join(work, ".nself", "trust", keysName), []byte("# attacker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, work, "commit", "-am", "attacker")
	gitIn(t, work, "tag", "origin/main")
	// A stale origin/HEAD and a refspec that excludes main.
	gitIn(t, work, "push", "origin", "HEAD:refs/heads/other")
	gitIn(t, work, "config", "remote.origin.fetch", "+refs/heads/other:refs/remotes/origin/other")
	gitIn(t, work, "fetch", "origin")
	gitIn(t, work, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/other")
	commit, err := g.PinDefault(ctx)
	if err != nil {
		t.Fatal(err)
	}
	data, found, err := g.Show(ctx, commit, trustRel+keysName)
	if err != nil || !found || string(data) != "# real\n" {
		t.Fatalf("pinned content = %q found=%v err=%v, want the remote's main", data, found, err)
	}
	// Origin's main moves on; the next pin follows it despite the narrowed refspec.
	gitIn(t, work, "checkout", "-q", "-B", "next", commit)
	if err := os.WriteFile(filepath.Join(work, ".nself", "trust", revokedName), []byte("# newer\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, work, "add", ".nself/trust/"+revokedName)
	gitIn(t, work, "commit", "-m", "newer")
	gitIn(t, work, "push", "origin", "HEAD:refs/heads/main")
	moved, err := g.PinDefault(ctx)
	if err != nil || moved == commit {
		t.Fatalf("pin after origin moved = %q (before %q), err %v", moved, commit, err)
	}
	if data, found, _ := g.Show(ctx, moved, trustRel+revokedName); !found || string(data) != "# newer\n" {
		t.Fatalf("revoked list at the new pin = %q found=%v", data, found)
	}
}

// TestGitCLIShowBoundsTheFileSize: a committed trust file of exactly the limit
// is read whole; one byte more is refused.
func TestGitCLIShowBoundsTheFileSize(t *testing.T) {
	work := gitRepo(t, "# k\n")
	dir := filepath.Join(work, ".nself", "trust")
	if err := os.WriteFile(filepath.Join(dir, keysName), make([]byte, trustFileLimit), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, revokedName), make([]byte, trustFileLimit+1), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, work, "add", ".")
	gitIn(t, work, "commit", "-m", "big")
	gitIn(t, work, "push", "origin", "HEAD:refs/heads/main")
	g := GitCLI{Root: work}
	commit, err := g.PinDefault(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if data, found, err := g.Show(context.Background(), commit, trustRel+keysName); err != nil || !found || len(data) != trustFileLimit {
		t.Fatalf("file of exactly the limit: len %d found %v err %v", len(data), found, err)
	}
	if _, found, err := g.Show(context.Background(), commit, trustRel+revokedName); err == nil || found {
		t.Fatalf("file one byte over the limit: found %v err %v, want a refusal", found, err)
	}
}
