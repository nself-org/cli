package secrets

// Purpose: prove saveStore replaces the store atomically.
// Inputs: a fake age that fails mid-write; a ci store hardlinked to release.
// Outputs: assertions that the live ciphertext and the linked store survive.
// Constraints: synthetic values only; the fake age needs a POSIX shell.
// SPORT: P7-TRUST-03 (Codex pre-merge must-fix 1 and 2).

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// TestSaveStoreKeepsCiphertextWhenAgeFails proves a failed encrypt never truncates the store.
func TestSaveStoreKeepsCiphertextWhenAgeFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake age is a shell script")
	}
	root, bin := t.TempDir(), t.TempDir()
	dir := filepath.Join(root, ".secrets")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "ci.age")
	original := []byte("ORIGINAL-CIPHERTEXT")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	// The fake age writes a partial file to its -o operand, then fails.
	script := "#!/bin/sh\nout=\nwhile [ $# -gt 0 ]; do [ \"$1\" = -o ] && out=$2; shift; done\nprintf partial > \"$out\"\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "age"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	store := &SecretStore{Secrets: map[string]SecretEntry{}, Recipients: []string{"age1" + strings.Repeat("q", 58)}}
	if err := saveStore(root, "ci", store); err == nil {
		t.Fatal("saveStore succeeded with a failing age")
	}
	after, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(original, after) {
		t.Fatalf("live ciphertext changed after a failed save: %q %v", after, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary file left behind: %v %v", entries, err)
	}
}

// TestRecipientsHardlinkLeavesReleaseUntouched proves a ci grant cannot rewrite a linked release store.
func TestRecipientsHardlinkLeavesReleaseUntouched(t *testing.T) {
	root, home, _ := trustFixture(t)
	trustStore(t, root, "release", "X")
	release := filepath.Join(root, ".secrets", "release.age")
	path := filepath.Join(root, ".secrets", "ci.age")
	if err := os.Link(release, path); err != nil {
		t.Skip("hardlinks unsupported:", err)
	}
	before, err := os.ReadFile(release)
	if err != nil {
		t.Fatal(err)
	}
	node := trustKey(t, filepath.Join(home, "node.key"))
	_, changed, err := Recipients(RecipientsRequest{ProjectRoot: root, File: "ci", Action: "add", Recipient: node}, func(string, map[string]string) error { return nil })
	if err != nil || !changed {
		t.Fatal("grant on ci failed", err)
	}
	after, err := os.ReadFile(release)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("grant on ci rewrote the hardlinked release store", err)
	}
}
