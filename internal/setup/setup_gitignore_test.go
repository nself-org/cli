package setup

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGitignoreFresh: a new project ignores .nself/ state but keeps
// .nself/ci.yaml committable, with the negation after the wildcard.
func TestGitignoreFresh(t *testing.T) {
	dir := t.TempDir()
	if err := ensureGitignore(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	wild, neg := strings.Index(got, "\n.nself/*\n"), strings.Index(got, "\n!.nself/ci.yaml\n")
	if wild < 0 || neg < 0 || neg < wild {
		t.Fatalf("want .nself/* then !.nself/ci.yaml, got:\n%s", got)
	}
	for _, l := range strings.Split(got, "\n") {
		if l == ".nself/" {
			t.Fatalf("a fresh .gitignore must not ignore the whole .nself/ directory:\n%s", got)
		}
	}
	// Idempotent: a second run appends nothing.
	if err := ensureGitignore(dir); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if string(again) != got {
		t.Fatalf("second run changed the file:\n%s", again)
	}
}

// TestGitignoreExistingLine: an existing `.nself/` directory line is never
// rewritten and no .nself entry is appended (the negation could not take
// effect under an excluded directory); other missing entries still are.
func TestGitignoreExistingLine(t *testing.T) {
	full := strings.Join([]string{".env", ".env.local", ".env.*.local", ".env.secrets", ".env.ai", ".volumes/", "logs/", "*.log", "node_modules/", ".DS_Store", "ssl/", ".nself/"}, "\n") + "\n"
	for name, body := range map[string]string{"complete": full, "only-nself": "/.nself/\n"} {
		dir := t.TempDir()
		p := filepath.Join(dir, ".gitignore")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := ensureGitignore(dir); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(p)
		if name == "complete" && string(got) != body {
			t.Fatalf("%s: file with every entry and .nself/ must stay byte-identical, got:\n%s", name, got)
		}
		if !strings.HasPrefix(string(got), body) {
			t.Fatalf("%s: existing content was rewritten:\n%s", name, got)
		}
		if strings.Contains(string(got[len(body):]), ".nself") {
			t.Fatalf("%s: .nself entries appended under an existing directory ignore:\n%s", name, got)
		}
	}
}

// TestGitignoreSSL (D-0163): ssl/ is ignored so certificate private keys
// cannot be committed. Only a whole line that ignores the directory counts as
// present; `ssl/.acme/` or a comment mentioning ssl/ does not. Runs twice to
// prove the append is idempotent.
func TestGitignoreSSL(t *testing.T) {
	cases := []struct {
		name, body string
		wantAppend bool
	}{
		{"fresh", "", true},
		{"acme-only", "ssl/.acme/\n", true},
		{"comment", "# keep ssl/ out of git\n", true},
		{"nested-path", "backend/ssl/\n", true},
		{"present", "ssl/\n", false},
		{"anchored", "/ssl/\n", false},
		{"bare", "ssl\n", false},
		{"leading-space", "  ssl/\n", true}, // D-0537: kills a leading-trim mutant
		{"glob-appends", "ssl/*\n", true},
		{"trailing-spaces", "ssl/  \n", false},
		{"leading-tab", "\tssl/\n", true},
		{"trailing-tab", "ssl/\t\n", true},
		{"crlf", "ssl/\r\n", false},
		{"negated-dir", "ssl/*\n!ssl/certificates/\n", true},
		{"negated-key", "ssl/**\n!ssl/**/privkey.pem\n", true},
		{"neg-name", "ssl/*\n!privkey.pem\n", true},
		{"neg-glob", "ssl/*\n!*.pem\n", true},
		{"neg-deep-glob", "ssl/**\n!**/privkey.pem\n", true},
		{"neg-subdir", "ssl/*\n!certificates/\n", true},
		{"neg-case", "ssl/*\n!SSL/certificates/\n", true},
		{"dir-neg-slash", "ssl/\n!ssl/\n", true},
		{"dir-neg-bare", "ssl/\n!ssl\n", true},
		{"dir-neg-anchored", "ssl/\n!/ssl\n", true},
		{"dir-neg-star", "ssl/\n!*\n", true},
		{"dir-neg-case", "ssl/\n!SSL\n", true},
		{"dir-neg-glob", "ssl/\n!s*\n", true},
		{"dir-neg-nul", "ssl/\n!ssl\x00/x\n", true},
		{"dir-neg-escaped", "ssl/\n!\\ssl\n", true},
		{"neg-nself", "ssl/\n!.nself/ci.yaml\n", false},
		{"neg-nself-anchored", "ssl/\n!/.nself/ci.yaml\n", false},
		{"negation-before", "!ssl/x\nssl/\n", false},
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
			appended := false
			for _, l := range strings.Split(string(first[len(tc.body):]), "\n") {
				appended = appended || l == "/ssl/"
			}
			if appended != tc.wantAppend {
				t.Fatalf("/ssl/ appended = %v, want %v:\n%s", appended, tc.wantAppend, first)
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

// TestGitignoreSSLGitSemantics (D-0163): asks real git whether keys are
// ignored after ensureGitignore, including past negations and odd spacing,
// and that a nested src/ssl/ stays tracked.
func TestGitignoreSSLGitSemantics(t *testing.T) {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH")
	}
	cases := []struct {
		name, body string
		nestedFree bool
		nestedBody string // written to ssl/.gitignore when set
	}{
		{"fresh", "", true, ""},
		{"negated", "ssl/*\n!ssl/certificates/\n", false, ""},
		{"leading-space", "  ssl/*\n", false, ""},
		{"plain", "ssl/\n", false, ""},
		{"nested-reinclude", "ssl/*\n", false, "!privkey.pem\n"},
		{"plain-glob", "ssl/**\n", false, ""},
		{"dir-neg-slash", "ssl/\n!ssl/\n", false, ""},
		{"dir-neg-nul", "ssl/\n!ssl\x00/x\n", false, ""},
		{"dir-neg-escaped", "ssl/\n!\\ssl\n", false, ""},
		{"neg-name", "ssl/*\n!privkey.pem\n", false, ""},
		{"neg-deep-glob", "ssl/**\n!**/privkey.pem\n", false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.body != "" {
				if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(tc.body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := ensureGitignore(dir); err != nil {
				t.Fatal(err)
			}
			if tc.nestedBody != "" {
				if err := os.MkdirAll(filepath.Join(dir, "ssl"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "ssl", ".gitignore"), []byte(tc.nestedBody), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			env := append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
			run := func(args ...string) int {
				cmd := exec.Command(gitBin, args...)
				cmd.Dir, cmd.Env = dir, env
				err := cmd.Run()
				if err == nil {
					return 0
				}
				if ee, ok := err.(*exec.ExitError); ok {
					return ee.ExitCode()
				}
				t.Fatal(err)
				return -1
			}
			if c := run("init", "-q"); c != 0 {
				t.Fatalf("git init exit %d", c)
			}
			for _, p := range []string{"ssl/privkey.pem", "ssl/certificates/example.test/privkey.pem"} {
				if c := run("check-ignore", "-q", "--no-index", p); c != 0 {
					t.Errorf("%s not ignored (exit %d)", p, c)
				}
			}
			if tc.nestedFree {
				if c := run("check-ignore", "-q", "--no-index", "services/api/src/ssl/tls.go"); c != 1 {
					t.Errorf("nested ssl/ path ignored (exit %d), want 1", c)
				}
			}
		})
	}
}
