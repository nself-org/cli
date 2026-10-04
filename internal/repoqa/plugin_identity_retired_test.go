package repoqa

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// retiredPluginIdentityLiterals are the strings that only the retired Q01
// plugin-identity client (D15, P7-PLUG-67) used. The client posted the
// project's PLUGIN_INTERNAL_SECRET to ping.nself.org on the first install of
// each plugin and kept per-host keys shared by every project. No non-test Go
// file may name any of them again.
var retiredPluginIdentityLiterals = []string{
	"/plugin/identity",
	"/plugin/token",
	"X-Plugin-Internal-Secret",
	"PLUGIN_TOKEN_ISSUER",
	"identity.key",
}

// scanRetiredLiterals returns one "name:line: literal" finding per retired
// literal found on each line of src. name is only used to label findings.
func scanRetiredLiterals(name string, src []byte) []string {
	var findings []string
	for i, line := range strings.Split(string(src), "\n") {
		for _, lit := range retiredPluginIdentityLiterals {
			if strings.Contains(line, lit) {
				findings = append(findings, fmt.Sprintf("%s:%d: %s", name, i+1, lit))
			}
		}
	}
	return findings
}

// TestPluginIdentityRetired walks the module and fails when any non-test,
// non-vendor, non-testdata Go file names a retired ping plugin-identity path,
// the X-Plugin-Internal-Secret header, PLUGIN_TOKEN_ISSUER or identity.key.
func TestPluginIdentityRetired(t *testing.T) {
	root := repoRoot(t)

	var violations []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			// Hidden dirs (.git, .claude worktrees, .github), vendor and
			// testdata are not first-party shipped code.
			if path != root && (strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		violations = append(violations, scanRetiredLiterals(filepath.ToSlash(rel), data)...)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if len(violations) > 0 {
		t.Fatalf("the Q01 plugin identity client is retired (D15); these files name it again:\n  %s",
			strings.Join(violations, "\n  "))
	}
}

// TestPluginIdentityScannerFixture runs the same scanner over a fixture that
// holds each retired literal once and expects exactly one finding per literal,
// so TestPluginIdentityRetired cannot pass because the scanner matches nothing.
func TestPluginIdentityScannerFixture(t *testing.T) {
	root := repoRoot(t)
	fixture := filepath.Join(root, "internal", "repoqa", "testdata", "plugin_identity_retired", "leak.go.txt")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	findings := scanRetiredLiterals("leak.go.txt", data)
	if len(findings) != len(retiredPluginIdentityLiterals) {
		t.Fatalf("expected %d findings, got %d: %v", len(retiredPluginIdentityLiterals), len(findings), findings)
	}
	for _, lit := range retiredPluginIdentityLiterals {
		n := 0
		for _, f := range findings {
			if strings.HasSuffix(f, ": "+lit) {
				n++
			}
		}
		if n != 1 {
			t.Errorf("literal %q: expected 1 finding, got %d", lit, n)
		}
	}
}
