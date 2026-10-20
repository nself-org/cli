package main

// licensed_platform_signatures_test.go: D-1311. No licensed platform producer
// exists (D-1286), so a licensed row must not carry platform_signatures, and
// catalog mode must check the registry entries it copies the key from with the
// same rules releases.json gets. Keys and signatures are placeholders.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A licensed releases.json row with platform_signatures and platform_checksums
// is refused (it used to exit 0 and write the key with checksums null).
func TestLicensedPlatformSignaturesRefused(t *testing.T) {
	dir := tree(t, "licensed", "claw")
	rel := decodeMap(t, readFile(t, filepath.Join(dir, "releases.json")))
	row := rel["releases"].(map[string]any)["claw"].(map[string]any)
	row["release_signature"] = fakeSig("source")
	row["platform_checksums"] = map[string]any{"darwin-arm64": strings.Repeat("0a", 32)}
	row["platform_signatures"] = map[string]any{"darwin-arm64": fakeSig("darwin-arm64")}
	b, err := json.Marshal(rel)
	if err != nil {
		t.Fatal(err)
	}
	writeReleases(t, dir, string(b))
	expectProblem(t, "claw: platform_signatures is not supported on a licensed row", tierArgs("licensed", dir, t.TempDir(), "")...)
}

// editRegistry rewrites one entry of a generated registry.json in place.
func editRegistry(t *testing.T, path, slug string, edit func(entry map[string]any)) {
	t.Helper()
	reg := decodeMap(t, readFile(t, path))
	edit(reg["plugins"].(map[string]any)[slug].(map[string]any))
	b, err := json.Marshal(reg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// Catalog mode runs the platform checks on the registry entries it reads, so a
// hand-edited registry cannot yield a catalog row the catalog schema rejects.
func TestCatalogModeChecksRegistryPlatformSignatures(t *testing.T) {
	cases := []struct {
		name, side, slug, want string
		edit                   func(e map[string]any)
	}{
		{"unknown platform", "free", "ai-cli", "ai-cli: platform_signatures.beos-ppc is not a platform", func(e map[string]any) {
			e["release_signature"] = fakeSig("source")
			e["platform_signatures"] = map[string]any{"beos-ppc": fakeSig("x")}
		}},
		{"bad alg", "free", "ai-cli", "platform_signatures.darwin-arm64 needs key_id, sig and alg ed25519", func(e map[string]any) {
			e["release_signature"] = fakeSig("source")
			bad := fakeSig("x")
			bad["alg"] = "rsa"
			e["platform_signatures"] = map[string]any{"darwin-arm64": bad}
		}},
		{"no source signature", "free", "ai-cli", "platform_signatures needs a release_signature", func(e map[string]any) {
			e["platform_signatures"] = map[string]any{"darwin-arm64": fakeSig("x")}
		}},
		{"licensed entry", "licensed", "claw", "claw: platform_signatures is not supported on a licensed row", func(e map[string]any) {
			e["release_signature"] = fakeSig("source")
			e["platform_signatures"] = map[string]any{"darwin-arm64": fakeSig("x")}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			free, lic := genBoth(t, t.TempDir())
			reg := map[string]string{"free": free, "licensed": lic}[c.side]
			// the fixtures give ai-cli all five platform checksums, so only the edit can make it invalid
			editRegistry(t, filepath.Join(reg, "registry.json"), c.slug, c.edit)
			expectProblem(t, c.want, "-mode", "catalog",
				"-free-registry", filepath.Join(free, "registry.json"),
				"-licensed-registry", filepath.Join(lic, "registry.json"),
				"-bundles", filepath.Join(td, "bundles.json"), "-out", t.TempDir())
		})
	}
}
