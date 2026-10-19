package repoqa

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var rootAllowedRe = regexp.MustCompile(`(?m)^\s*ALLOWED="([^"]+)"`)

// TestTrackedRootEntriesAreOnHygieneAllowlist keeps the Clean Root allowlist in
// .github/workflows/hygiene.yml honest. The hygiene check fails when git tracks
// a root entry the ALLOWED list does not name (D-1224: .nself/ci.yaml from
// P7-CI-18 was tracked without an allowlist entry, so the check went red on main
// and on every PR). Running the same comparison here fails locally, before push.
// It is skipped outside a git checkout.
func TestTrackedRootEntriesAreOnHygieneAllowlist(t *testing.T) {
	root := repoRoot(t)
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Skip("not a git checkout")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	data, err := os.ReadFile(filepath.Join(root, ".github/workflows/hygiene.yml"))
	if err != nil {
		t.Fatal(err)
	}
	m := rootAllowedRe.FindSubmatch(data)
	if m == nil {
		t.Fatal(`hygiene.yml has no ALLOWED="..." root allowlist`)
	}
	allowed := map[string]bool{}
	for _, name := range strings.Fields(string(m[1])) {
		allowed[name] = true
	}

	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	seen := map[string]bool{}
	for _, path := range strings.Split(string(out), "\x00") {
		if path == "" {
			continue
		}
		top := strings.SplitN(path, "/", 2)[0]
		if seen[top] || allowed[top] {
			continue
		}
		seen[top] = true
		t.Errorf("tracked root entry %q is not on the Clean Root allowlist in .github/workflows/hygiene.yml", top)
	}
}
