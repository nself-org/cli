package main

import (
	"path/filepath"
	"sort"
	"testing"
)

func TestGoldenRegistriesAndCatalog(t *testing.T) {
	dir := t.TempDir()
	free, lic := genBoth(t, dir)
	golden(t, filepath.Join(td, "free", "registry.json"), readFile(t, filepath.Join(free, "registry.json")))
	golden(t, filepath.Join(td, "licensed", "registry.json"), readFile(t, filepath.Join(lic, "registry.json")))
	out := filepath.Join(dir, "cat")
	mustGen(t, "-mode", "catalog", "-free-registry", filepath.Join(td, "free", "registry.json"),
		"-licensed-registry", filepath.Join(td, "licensed", "registry.json"), "-bundles", filepath.Join(td, "bundles.json"),
		"-out", out, "-plugins-ref", "plugins@fixture", "-bundles-ref", "bundles@fixture", "-generated-at", "2026-10-05T00:00:00Z")
	golden(t, filepath.Join(td, "golden", "catalog.json"), readFile(t, filepath.Join(out, "catalog.json")))
	golden(t, filepath.Join(td, "golden", "counts.json"), readFile(t, filepath.Join(out, "counts.json")))
}

// additive are the registry keys this tool adds to the basis shape; released
// CLIs ignore them (decode test).
var additive = map[string]bool{"catalog": true, "release_signature": true}

// TestKeySetMatchesBasis pins the entry key set to the real basis registries:
// every generated key is a basis key (of either registry: one decoder reads both)
// or one of the two additive keys, and every
// basis key of the tier the manifests can supply is produced by at least one entry.
func TestKeySetMatchesBasis(t *testing.T) {
	basis := decodeMap(t, readFile(t, filepath.Join(td, "basis-keys.json")))
	union := map[string]bool{} // one decoder reads both registries, so either basis key set is valid in both
	for _, tier := range []string{"free", "licensed"} {
		for _, k := range basis[tier].([]any) {
			union[k.(string)] = true
		}
	}
	for _, tier := range []string{"free", "licensed"} {
		want := map[string]bool{}
		for _, k := range basis[tier].([]any) {
			want[k.(string)] = true
		}
		delete(want, "security_always_free") // no manifest v2 home; no released CLI reads it
		got := map[string]bool{}
		reg := decodeMap(t, readFile(t, filepath.Join(td, tier, "registry.json")))
		for _, e := range reg["plugins"].(map[string]any) {
			for k := range e.(map[string]any) {
				got[k] = true
			}
		}
		for k := range got {
			if !union[k] && !additive[k] {
				t.Errorf("%s: generated key %q is not in the basis registry and not additive", tier, k)
			}
		}
		var missing []string
		for k := range want {
			if !got[k] {
				missing = append(missing, k)
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			t.Errorf("%s: basis keys never produced by the fixture: %v", tier, missing)
		}
	}
}
