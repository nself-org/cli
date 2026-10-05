package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeReleases(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "releases.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestVersionDisagreementIsNoted(t *testing.T) {
	dir := tree(t, "licensed", "claw") // manifest 1.1.2
	manifest := filepath.Join(dir, "claw", "plugin.json")
	b := strings.Replace(string(readFile(t, manifest)), `"version": "1.1.2"`, `"version": "1.1.12"`, 1)
	if err := os.WriteFile(manifest, []byte(b), 0o644); err != nil {
		t.Fatal(err)
	}
	code, se := gen(t, tierArgs("licensed", dir, t.TempDir(), "")...)
	if code != 0 || !strings.Contains(se, "claw: manifest version 1.1.12 differs from released version 1.1.2") {
		t.Fatalf("exit %d\n%s", code, se)
	}
}

func TestConflictingMinVersionKeysAreRefused(t *testing.T) {
	dir := tree(t, "free", "ai-cli")
	manifest := filepath.Join(dir, "ai-cli", "plugin.json")
	b := strings.Replace(string(readFile(t, manifest)), `"minNselfVersion": "1.2.8",`, `"minNselfVersion": "1.2.8", "min_nself_version": "1.3.0",`, 1)
	if err := os.WriteFile(manifest, []byte(b), 0o644); err != nil {
		t.Fatal(err)
	}
	expectProblem(t, `minNselfVersion "1.2.8" conflicts with min_nself_version "1.3.0"`, tierArgs("free", dir, t.TempDir(), "")...)
	// equal values are fine
	b = strings.Replace(b, `"1.3.0"`, `"1.2.8"`, 1)
	if err := os.WriteFile(manifest, []byte(b), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGen(t, tierArgs("free", dir, t.TempDir(), "")...)
}

func TestSkippedDirectoriesAreNotedAndErrorsNameThePath(t *testing.T) {
	dir := tree(t, "free", "ai-cli")
	if err := os.MkdirAll(filepath.Join(dir, "_template"), 0o755); err != nil {
		t.Fatal(err)
	}
	code, se := gen(t, tierArgs("free", dir, t.TempDir(), "")...)
	if code != 0 || !strings.Contains(se, "skipped directory "+filepath.Join(dir, "_template")) {
		t.Fatalf("exit %d\n%s", code, se)
	}
	if err := os.WriteFile(filepath.Join(dir, "ai-cli", "plugin.json"), []byte(`{"name":`), 0o644); err != nil {
		t.Fatal(err)
	}
	expectProblem(t, filepath.Join(dir, "ai-cli", "plugin.json")+":", tierArgs("free", dir, t.TempDir(), "")...)
}

func TestRequireReleasedFailsOnAnUnreleasedManifest(t *testing.T) {
	for _, c := range []struct{ tier, from, slug string }{{"free", "free", "ai-cli"}, {"licensed", "licensed", "claw"}} {
		dir := tree(t, c.from, c.slug)
		writeReleases(t, dir, `{"schema_version":1,"releases":{}}`)
		args := tierArgs(c.tier, dir, t.TempDir(), "")
		if code, se := gen(t, args...); code != 0 || !strings.Contains(se, "note: "+c.slug+" has no row") {
			t.Fatalf("%s default: exit %d\n%s", c.tier, code, se)
		}
		expectProblem(t, c.slug+": no row in releases.json (-require-released)", append(args, "-require-released")...)
	}
}

func TestReleaseBinariesMustCoverWhatTheManifestRequires(t *testing.T) {
	dir := tree(t, "free", "sentry-cli")
	sig := strings.Repeat("a", 64)
	writeReleases(t, dir, `{"schema_version":1,"releases":{"sentry-cli":{"version":"1.0.0","sha256":"`+sig+`","release_signature":null,"binaries":["nself-sentry"]}}}`)
	expectProblem(t, "sentry-cli: the manifest requires binaries nself-sentry-server but the release lists nself-sentry", tierArgs("free", dir, t.TempDir(), "")...)
	writeReleases(t, dir, `{"schema_version":1,"releases":{"sentry-cli":{"version":"1.0.0","sha256":"`+sig+`","release_signature":null,"binaries":["Sentry"]}}}`)
	expectProblem(t, "binaries entry Sentry must match", tierArgs("free", dir, t.TempDir(), "")...)
}
