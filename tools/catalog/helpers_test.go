package main

// helpers_test.go: shared test helpers and the fixture layout.
//
// Fixtures live in testdata/: free/ and licensed/ (one directory per plugin plus
// releases.json and the golden registry.json), bundles.json, golden/ (catalog.json,
// counts.json) and basis-keys.json (the key sets of the real basis registries).
// Run `go test ./tools/catalog -update` to rewrite every golden file.

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

const td = "testdata"

// gen runs the tool in-process and returns its exit code and stderr.
func gen(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var so, se bytes.Buffer
	code := run(args, &so, &se)
	return code, se.String()
}

func mustGen(t *testing.T, args ...string) {
	t.Helper()
	if code, se := gen(t, args...); code != 0 {
		t.Fatalf("catalog %v: exit %d\n%s", args, code, se)
	}
}

func readFile(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// tierArgs builds the flags of one tier run over the fixtures.
func tierArgs(tier, tree, out, peer string, extra ...string) []string {
	a := []string{"-mode", tier, "-plugins", tree, "-releases", filepath.Join(tree, "releases.json"),
		"-bundles", filepath.Join(td, "bundles.json"), "-out", out, "-plugins-ref", "plugins@fixture",
		"-bundles-ref", "bundles@fixture", "-generated-at", "2026-10-05T00:00:00Z"}
	if peer != "" {
		a = append(a, "-peer-registry", peer)
	}
	return append(a, extra...)
}

// genBoth writes both registries to dir/{free,licensed}: a first pass without
// peers to learn each side's slugs, a second with them (tier_pair).
func genBoth(t *testing.T, dir string) (free, lic string) {
	t.Helper()
	p1f, p1l := filepath.Join(dir, "p1f"), filepath.Join(dir, "p1l")
	mustGen(t, tierArgs("free", filepath.Join(td, "free"), p1f, "")...)
	mustGen(t, tierArgs("licensed", filepath.Join(td, "licensed"), p1l, "")...)
	free, lic = filepath.Join(dir, "free"), filepath.Join(dir, "licensed")
	mustGen(t, tierArgs("free", filepath.Join(td, "free"), free, filepath.Join(p1l, "registry.json"))...)
	mustGen(t, tierArgs("licensed", filepath.Join(td, "licensed"), lic, filepath.Join(p1f, "registry.json"))...)
	return free, lic
}

// golden compares got with the file at path, or rewrites it under -update.
func golden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if want := readFile(t, path); !bytes.Equal(want, got) {
		t.Fatalf("%s differs from the generator output (run go test ./tools/catalog -update and review the diff)", path)
	}
}

func decodeMap(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}
