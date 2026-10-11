package setup

import (
	"os"
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
		{"contents", "  ssl/*  \n", false},
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
				appended = appended || l == "ssl/"
			}
			if appended != tc.wantAppend {
				t.Fatalf("ssl/ appended = %v, want %v:\n%s", appended, tc.wantAppend, first)
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
