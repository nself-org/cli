package secrets

// Purpose: keep the recipient-store lock sidecar out of version control.
// Inputs: temp project trees, including a .gitignore from before *.lock.
// Outputs: assertions on .secrets/.gitignore content and git's verdict.
// Constraints: no committed private identities.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnsureGitignoreCoversLocks proves new and legacy files both ignore locks.
func TestEnsureGitignoreCoversLocks(t *testing.T) {
	fresh := t.TempDir()
	if err := ensureGitignore(fresh); err != nil {
		t.Fatal(err)
	}
	legacy := t.TempDir()
	old := "# Never commit decrypted secrets\n*.json\n*.tmp"
	if err := os.WriteFile(filepath.Join(legacy, ".gitignore"), []byte(old), 0644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // second pass proves idempotence
		if err := ensureGitignore(legacy); err != nil {
			t.Fatal(err)
		}
	}
	for name, dir := range map[string]string{"fresh": fresh, "legacy": legacy} {
		b, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
		if err != nil {
			t.Fatal(err)
		}
		for _, rule := range []string{"*.json", "*.tmp", "*.lock"} {
			if strings.Count(string(b), rule+"\n") != 1 {
				t.Errorf("%s .gitignore: want exactly one %q in %q", name, rule, b)
			}
		}
	}
}

// TestRecipientsChangeLeavesNoUntrackedLock proves a recipient change in a
// project whose .gitignore predates *.lock does not surface the sidecar to git.
func TestRecipientsChangeLeavesNoUntrackedLock(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root, home, _ := trustFixture(t)
	trustStore(t, root, "ci", "FOO")
	node := trustKey(t, filepath.Join(home, "node.key"))
	gi := filepath.Join(root, ".secrets", ".gitignore")
	if err := os.WriteFile(gi, []byte("# Never commit decrypted secrets\n*.json\n*.tmp\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	req := RecipientsRequest{ProjectRoot: root, File: "ci", Action: "add", Recipient: node}
	if _, _, err := Recipients(req, func(string, map[string]string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(root, ".secrets", "ci.age.lock")
	if _, err := os.Stat(lock); err != nil {
		t.Fatalf("expected lock sidecar to exist: %v", err)
	}
	if err := exec.Command("git", "-C", root, "check-ignore", "-q", lock).Run(); err != nil {
		t.Fatalf("lock sidecar is not git-ignored: %v", err)
	}
}
