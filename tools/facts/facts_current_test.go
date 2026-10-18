package main

import (
	"path/filepath"
	"testing"
)

// repoRoot is the repository root relative to this package.
var repoRoot = filepath.Join("..", "..")

// TestFactsCurrent fails when .github/facts.json differs from a fresh build,
// for example after a VERSION bump or a registry change without `make cmd-inventory`.
func TestFactsCurrent(t *testing.T) {
	if code, msg := exec(repoRoot, true, ""); code != 0 {
		t.Fatalf("facts.json is stale or sources are invalid (exit %d): %s", code, msg)
	}
}

// TestReadmeFactsCurrent runs `-check -readme` on the repo README. It is a
// no-op while the README has no facts blocks and gates once they exist.
func TestReadmeFactsCurrent(t *testing.T) {
	if code, msg := exec(repoRoot, true, filepath.Join(repoRoot, "README.md")); code != 0 {
		t.Fatalf("README facts blocks are stale (exit %d): %s", code, msg)
	}
}
