package setup

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitCheckIgnore returns git check-ignore's exit code for path in a fresh
// repo at dir: 0 ignored, 1 not ignored. Global and system config are off so
// a developer's core.excludesFile cannot change the answer. The literal
// /dev/null (not os.DevNull, which is NUL) is what Git for Windows accepts.
func gitCheckIgnore(t *testing.T, dir, path string) int {
	t.Helper()
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH")
	}
	run := func(args ...string) int {
		cmd := exec.Command(gitBin, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		err := cmd.Run()
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		if err != nil {
			t.Fatal(err)
		}
		return 0
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		if c := run("init", "-q"); c != 0 {
			t.Fatalf("git init exit %d", c)
		}
	}
	return run("check-ignore", "-q", "--no-index", path)
}

// TestGitignoreCIYamlGitSemantics (D-0537): whatever order a hand-edited
// .gitignore holds the two .nself lines in, after ensureGitignore real git
// keeps .nself/ci.yaml committable and still ignores .nself/ state. The
// control proves the probe sees the bug: a negation before the wildcard
// leaves ci.yaml ignored when nothing repairs it.
func TestGitignoreCIYamlGitSemantics(t *testing.T) {
	control := t.TempDir()
	if err := os.WriteFile(filepath.Join(control, ".gitignore"), []byte("!.nself/ci.yaml\n.nself/*\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if c := gitCheckIgnore(t, control, ".nself/ci.yaml"); c != 0 {
		t.Fatalf("control: ci.yaml after a later wildcard should be ignored, exit %d", c)
	}

	cases := []struct{ name, body string }{
		{"fresh", ""},
		{"neg-only", "!.nself/ci.yaml\n"},
		{"neg-before-wild", "!.nself/ci.yaml\n.nself/*\n"},
		{"wild-then-neg", ".nself/*\n!.nself/ci.yaml\n"},
		{"neg-wild-neg-wild", "!.nself/ci.yaml\n.nself/*\n!.nself/ci.yaml\n.nself/*\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, ".gitignore")
			if tc.body != "" {
				if err := os.WriteFile(p, []byte(tc.body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := ensureGitignore(dir); err != nil {
				t.Fatal(err)
			}
			first, _ := os.ReadFile(p)
			if !strings.HasPrefix(string(first), tc.body) {
				t.Fatalf("existing content was rewritten:\n%s", first)
			}
			if c := gitCheckIgnore(t, dir, ".nself/ci.yaml"); c != 1 {
				t.Errorf(".nself/ci.yaml ignored (exit %d), want committable:\n%s", c, first)
			}
			if c := gitCheckIgnore(t, dir, ".nself/state/run.json"); c != 0 {
				t.Errorf(".nself state not ignored (exit %d):\n%s", c, first)
			}
			if err := ensureGitignore(dir); err != nil {
				t.Fatal(err)
			}
			if again, _ := os.ReadFile(p); string(again) != string(first) {
				t.Fatalf("second run changed the file:\n%s", again)
			}
		})
	}
}

// TestGitignoreCIYamlKeptOrder (D-0537): a file already ending wildcard then
// negation gets no .nself line appended.
func TestGitignoreCIYamlKeptOrder(t *testing.T) {
	dir := t.TempDir()
	body := ".nself/*\n!.nself/ci.yaml\n"
	p := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureGitignore(dir); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if strings.Contains(string(got[len(body):]), ".nself") {
		t.Fatalf(".nself line appended to an already correct order:\n%s", got)
	}
}
