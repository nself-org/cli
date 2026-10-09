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
	full := strings.Join([]string{".env", ".env.local", ".env.*.local", ".env.secrets", ".env.ai", ".volumes/", "logs/", "*.log", "node_modules/", ".DS_Store", ".nself/"}, "\n") + "\n"
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
