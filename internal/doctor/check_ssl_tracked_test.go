package doctor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// sslRepo makes a temp git repo with the given root .gitignore and returns
// its path plus a git runner. Global and system git config are off so the
// developer's own excludes cannot change the result.
func sslRepo(t *testing.T, gitignore string) (string, func(args ...string)) {
	t.Helper()
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(gitBin, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	if gitignore != "" {
		writeSSLFile(t, dir, ".gitignore", gitignore)
	}
	return dir, run
}

// writeSSLFile writes a fake fixture file (never real key material).
func writeSSLFile(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

const fakeKey = "fake-test-key-not-a-real-secret\n"

// TestCheckSSLTracked (D-0459): the check asks git, so it warns on a missing
// ignore, a negation that re-includes keys, and keys already tracked, and
// passes on a correctly ignored project.
func TestCheckSSLTracked(t *testing.T) {
	cases := []struct {
		name, gitignore string
		track           []string // files force-added to the index
		wantStatus      string
		wantIn          []string
		wantFix         bool
	}{
		{"ignored", "/ssl/\n.env.secrets\n", nil, "pass", []string{"ignored by git"}, false},
		{"no-gitignore", "", nil, "warn", []string{"not ignored by git", "ssl/privkey.pem", ".env.secrets"}, false},
		{"secrets-only", ".env.secrets\n", nil, "warn", []string{"ssl/certificates/x/privkey.pem"}, false},
		{"negated-certs", "ssl/*\n!ssl/certificates/\n.env.secrets\n", nil, "warn", []string{"ssl/certificates/x/privkey.pem"}, false},
		{"tracked-key", "/ssl/\n.env.secrets\n", []string{"ssl/certificates/example.test/privkey.pem"}, "warn",
			[]string{"1 tracked file", "ssl/certificates/example.test/privkey.pem", "rotate", "git rm --cached -r ssl/"}, true},
		{"tracked-secrets", "/ssl/\n.env.secrets\n", []string{".env.secrets"}, "warn", []string{"1 tracked file", ".env.secrets"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, git := sslRepo(t, tc.gitignore)
			for _, f := range tc.track {
				writeSSLFile(t, dir, f, fakeKey)
				git("add", "-f", "--", f)
			}
			r := CheckSSLTracked(context.Background(), dir)
			if r.Section != "security" || r.Name != sslTrackedCheckName {
				t.Fatalf("section/name = %q/%q", r.Section, r.Name)
			}
			if r.Status != tc.wantStatus {
				t.Fatalf("status %q, want %q: %s", r.Status, tc.wantStatus, r.Message)
			}
			for _, w := range tc.wantIn {
				if !strings.Contains(r.Message, w) {
					t.Errorf("message lacks %q: %s", w, r.Message)
				}
			}
			if (r.FixCmd != "") != tc.wantFix {
				t.Errorf("FixCmd = %q, want set=%v", r.FixCmd, tc.wantFix)
			}
		})
	}
}

// TestCheckSSLTrackedSubdir: a project in a subdirectory of the repo (the
// monorepo backend/ layout) is judged relative to that directory.
func TestCheckSSLTrackedSubdir(t *testing.T) {
	root, git := sslRepo(t, "")
	writeSSLFile(t, root, "backend/.gitignore", "/ssl/\n.env.secrets\n")
	proj := filepath.Join(root, "backend")
	if r := CheckSSLTracked(context.Background(), proj); r.Status != "pass" {
		t.Fatalf("subdir with its own ignore: %s %s", r.Status, r.Message)
	}
	writeSSLFile(t, root, "backend/ssl/privkey.pem", fakeKey)
	git("add", "-f", "--", "backend/ssl/privkey.pem")
	r := CheckSSLTracked(context.Background(), proj)
	if r.Status != "warn" || !strings.Contains(r.Message, "ssl/privkey.pem") {
		t.Fatalf("tracked key under the subdir not reported: %s %s", r.Status, r.Message)
	}
}

// TestCheckSSLTrackedNotARepo: outside a git work tree the check is skipped.
func TestCheckSSLTrackedNotARepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	ceiling, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CEILING_DIRECTORIES", ceiling)
	dir := t.TempDir()
	r := CheckSSLTracked(context.Background(), dir)
	if r.Status != "pass" || !strings.Contains(r.Message, "skipped") {
		t.Fatalf("got %s %s", r.Status, r.Message)
	}
}

// TestCheckSSLTrackedInSecurityChecks: the check is wired into the
// security section that `nself doctor --deep` runs.
func TestCheckSSLTrackedInSecurityChecks(t *testing.T) {
	dir, _ := sslRepo(t, "/ssl/\n.env.secrets\n")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, r := range SecurityChecks(ctx, dir) {
		if r.Name == sslTrackedCheckName {
			return
		}
	}
	t.Fatalf("%s missing from SecurityChecks", sslTrackedCheckName)
}

// TestCheckSSLTrackedGitErrors: a git failure other than "not a git
// repository" warns instead of passing as skipped, so a dubious-ownership or
// broken repo cannot hide tracked keys. A stub git script fakes the exits.
func TestCheckSSLTrackedGitErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub git is a shell script")
	}
	cases := []struct{ name, script, want string }{
		{"not-a-repo", `echo "fatal: not a git repository (or any of the parent directories): .git" >&2; exit 128`, "skipped"},
		{"dubious-ownership", `echo "fatal: detected dubious ownership in repository at '/srv/app'" >&2; exit 128`, "git rev-parse failed (exit 128): fatal: detected dubious ownership"},
		{"check-ignore-and-ls-files-fail", `if [ "$1" = rev-parse ]; then echo true; exit 0; fi; echo "fatal: index file corrupt" >&2; exit 128`, "git check-ignore ssl/privkey.pem failed (exit 128): fatal: index file corrupt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bin := filepath.Join(t.TempDir(), "git")
			if err := os.WriteFile(bin, []byte("#!/bin/sh\n"+tc.script+"\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			old := sslGitBin
			sslGitBin = func() (string, error) { return bin, nil }
			defer func() { sslGitBin = old }()
			r := CheckSSLTracked(context.Background(), t.TempDir())
			if !strings.Contains(r.Message, tc.want) {
				t.Fatalf("message lacks %q: %s %s", tc.want, r.Status, r.Message)
			}
			wantStatus := "warn"
			if tc.want == "skipped" {
				wantStatus = "pass"
			}
			if r.Status != wantStatus {
				t.Fatalf("status %q, want %q: %s", r.Status, wantStatus, r.Message)
			}
			if tc.name == "check-ignore-and-ls-files-fail" && !strings.Contains(r.Message, "git ls-files failed (exit 128)") {
				t.Fatalf("ls-files failure not reported: %s", r.Message)
			}
		})
	}
}
