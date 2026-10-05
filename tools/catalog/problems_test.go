package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tree copies the named fixture plugins and a releases.json into a temp dir.
func tree(t *testing.T, from string, slugs ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, s := range slugs {
		if err := os.MkdirAll(filepath.Join(dir, s), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, s, "plugin.json"), readFile(t, filepath.Join(td, from, s, "plugin.json")), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "releases.json"), readFile(t, filepath.Join(td, from, "releases.json")), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func expectProblem(t *testing.T, want string, args ...string) {
	t.Helper()
	code, se := gen(t, args...)
	if code != 2 || !strings.Contains(se, want) {
		t.Fatalf("want exit 2 mentioning %q, got %d:\n%s", want, code, se)
	}
}

func TestTierMismatchIsRefused(t *testing.T) {
	// a licensed manifest in the free tree and a free one in the licensed tree: a plugin never moves tier here
	expectProblem(t, "does not belong in the free tree", tierArgs("free", tree(t, "licensed", "claw"), t.TempDir(), "")...)
	expectProblem(t, "does not belong in the licensed tree", tierArgs("licensed", tree(t, "free", "ai-cli"), t.TempDir(), "")...)
}

func TestNameDirectoryMismatchIsRefused(t *testing.T) {
	dir := tree(t, "free", "ai-cli")
	if err := os.Rename(filepath.Join(dir, "ai-cli"), filepath.Join(dir, "ai")); err != nil {
		t.Fatal(err)
	}
	expectProblem(t, `manifest name "ai-cli" differs from its directory`, tierArgs("free", dir, t.TempDir(), "")...)
}

func TestUnreleasedManifestIsNotedAndOrphanReleaseRefused(t *testing.T) {
	dir := tree(t, "free", "ai-cli")
	if err := os.WriteFile(filepath.Join(dir, "releases.json"), []byte(`{"schema_version":1,"releases":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, se := gen(t, tierArgs("free", dir, t.TempDir(), "")...)
	if code != 0 || !strings.Contains(se, "note: ai-cli has no row in releases.json") {
		t.Fatalf("unreleased manifest: exit %d\n%s", code, se)
	}
	expectProblem(t, "releases.json has a row but the tree has no such directory", tierArgs("free", tree(t, "free", "ai-cli"), t.TempDir(), "")...)
}

func TestBadReleasesAreRefused(t *testing.T) {
	dir := tree(t, "free", "ai-cli")
	for body, want := range map[string]string{
		`{"schema_version":1,"releases":{"ai-cli":{"version":"1.0.0","sha256":"abc","release_signature":null}}}`:                                                             "sha256 must be 64 lowercase hex",
		`{"schema_version":1,"releases":{"ai-cli":{"version":"1.0.0","sha256":"` + strings.Repeat("a", 64) + `","release_signature":{"key_id":"k","alg":"rsa","sig":"s"}}}}`: "release_signature needs key_id, sig and alg ed25519",
		`{"schema_version":2,"releases":{}}`:                                           "schema_version 2 is not supported",
		`{"schema_version":1,"releases":{"ai-cli":{"version":"1","sha256":"","x":1}}}`: "unknown field",
	} {
		if err := os.WriteFile(filepath.Join(dir, "releases.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		expectProblem(t, want, tierArgs("free", dir, t.TempDir(), "")...)
	}
}

func catalogArgs(free, lic, bundles string) []string {
	return []string{"-mode", "catalog", "-free-registry", free, "-licensed-registry", lic, "-bundles", bundles, "-out", ""}
}

func TestCatalogRefusesBrokenJoins(t *testing.T) {
	free, lic := genBoth(t, t.TempDir())
	fr, lr := filepath.Join(free, "registry.json"), filepath.Join(lic, "registry.json")
	b := filepath.Join(t.TempDir(), "bundles.json")
	write := func(body string) {
		if err := os.WriteFile(b, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	args := func() []string { a := catalogArgs(fr, lr, b); a[len(a)-1] = t.TempDir(); return a }
	write(`{"schema_version":"2.0.0","bundles":{"x":{"display":"X","tier":"paid","plugins":["ghost"]}}}`)
	expectProblem(t, "bundle x lists ghost, which is not in the licensed registry", args()...)
	write(`{"schema_version":"2.0.0","bundles":{"x":{"display":"X","tier":"gold","plugins":[]}}}`)
	expectProblem(t, "tier \"gold\" is not free or paid", args()...)
	// a registry without catalog blocks (the committed basis files) cannot be joined
	write(`{"schema_version":"2.0.0","bundles":{}}`)
	old := filepath.Join(t.TempDir(), "old.json")
	if err := os.WriteFile(old, []byte(`{"plugins":{"a":{"version":"1.0.0","checksum":"x"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	a := catalogArgs(old, lr, b)
	a[len(a)-1] = t.TempDir()
	expectProblem(t, "registry entry has no catalog block", a...)
	// a shared slug without tier_pair on both sides
	un := filepath.Join(t.TempDir(), "unpaired.json")
	body := strings.Replace(string(readFile(t, lr)), `"tier_pair": true`, `"tier_pair": false`, 1)
	if err := os.WriteFile(un, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	a = catalogArgs(fr, un, b)
	a[len(a)-1] = t.TempDir()
	expectProblem(t, "present in both registries without tier_pair on both sides", a...)
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{{}, {"-out", "x"}, {"-mode", "bogus", "-out", "x"}, {"-mode", "free", "-out", "x"},
		{"-mode", "catalog", "-out", "x"}, {"-mode", "free", "-out", "x", "-generated-at", "now"}, {"-nope"}} {
		if code, _ := gen(t, args...); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
	}
}

func TestPartialWritesWhatLoadsButStillFails(t *testing.T) {
	dir := tree(t, "free", "ai-cli", "search")
	if err := os.Rename(filepath.Join(dir, "search"), filepath.Join(dir, "other")); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	code, _ := gen(t, tierArgs("free", dir, out, "", "-partial")...)
	if code != 2 {
		t.Fatalf("-partial with a problem: exit %d, want 2", code)
	}
	if !strings.Contains(string(readFile(t, filepath.Join(out, "registry.json"))), `"ai-cli"`) {
		t.Fatal("-partial must still write the plugins that loaded")
	}
}
