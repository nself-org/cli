package main

// platform_signatures_test.go: contract:plugins.release-signature v1.1, the
// registry-side projection of platform_signatures (P7-PLUG-85). All keys and
// signatures here are generated placeholders, never real material.

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

func fakeSig(label string) map[string]any {
	return map[string]any{"key_id": "fixture-key-1", "alg": "ed25519",
		"sig": base64.StdEncoding.EncodeToString([]byte("fixture-" + label))}
}

// signedTree is a free tree of ai-cli whose release row carries release_signature
// and the given platform_signatures (the row already has all five platform_checksums).
func signedTree(t *testing.T, sigs map[string]any) string {
	t.Helper()
	dir := tree(t, "free", "ai-cli")
	rel := decodeMap(t, readFile(t, filepath.Join(dir, "releases.json")))
	row := rel["releases"].(map[string]any)["ai-cli"].(map[string]any)
	row["release_signature"] = fakeSig("source")
	if sigs != nil {
		row["platform_signatures"] = sigs
	}
	b, err := json.Marshal(rel)
	if err != nil {
		t.Fatal(err)
	}
	writeReleases(t, dir, string(b))
	return dir
}

func TestPlatformSignaturesProjected(t *testing.T) {
	want := map[string]any{"darwin-arm64": fakeSig("darwin-arm64"), "linux-amd64": fakeSig("linux-amd64")}
	root := t.TempDir()
	free, lic := filepath.Join(root, "free"), filepath.Join(root, "licensed")
	mustGen(t, tierArgs("free", signedTree(t, want), free, "")...)
	mustGen(t, tierArgs("licensed", tree(t, "licensed", "claw"), lic, "")...)

	entry := decodeMap(t, readFile(t, filepath.Join(free, "registry.json")))["plugins"].(map[string]any)["ai-cli"].(map[string]any)
	if got := entry["platform_signatures"]; !reflect.DeepEqual(got, any(want)) {
		t.Fatalf("registry entry platform_signatures = %v, want %v", got, want)
	}
	if _, nested := entry["checksums"].(map[string]any)["platform_signatures"]; nested {
		t.Fatal("platform_signatures must sit at the entry level, never inside checksums")
	}

	bundles := filepath.Join(root, "bundles.json")
	if err := os.WriteFile(bundles, []byte(`{"schema_version":"2.0.0","bundles":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "cat")
	mustGen(t, "-mode", "catalog", "-free-registry", filepath.Join(free, "registry.json"),
		"-licensed-registry", filepath.Join(lic, "registry.json"), "-bundles", bundles, "-out", out)
	var row map[string]any
	for _, p := range decodeMap(t, readFile(t, filepath.Join(out, "catalog.json")))["plugins"].([]any) {
		if p.(map[string]any)["slug"] == "ai-cli" {
			row = p.(map[string]any)
		}
	}
	if row == nil || !reflect.DeepEqual(row["platform_signatures"], any(want)) {
		t.Fatalf("catalog row platform_signatures = %v, want %v", row["platform_signatures"], want)
	}
	if bytes := readFile(t, filepath.Join(out, "catalog.json")); strings.Count(string(bytes), `"platform_signatures"`) != 1 {
		t.Fatal("platform_signatures must appear on the signed row only")
	}

	// the generated schemas describe the key
	out2 := filepath.Join(root, "releases.json")
	if err := os.WriteFile(out2, readFile(t, filepath.Join(signedTree(t, want), "releases.json")), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ schema, doc string }{
		{"catalog.v1.schema.json", filepath.Join(out, "catalog.json")},
		{"releases.v1.schema.json", out2},
	} {
		if err := validateAgainst(t, c.schema, decodeMap(t, readFile(t, c.doc))); err != nil {
			t.Errorf("%s does not validate against %s: %v", c.doc, c.schema, err)
		}
	}

	// the schema pins alg to ed25519 for a platform signature
	rel := decodeMap(t, readFile(t, out2))
	rel["releases"].(map[string]any)["ai-cli"].(map[string]any)["platform_signatures"].(map[string]any)["darwin-arm64"].(map[string]any)["alg"] = "rsa"
	if validateAgainst(t, "releases.v1.schema.json", rel) == nil {
		t.Error("releases schema accepted platform_signatures alg rsa")
	}
	// and rejects a null value or a name that is not a platform
	row := rel["releases"].(map[string]any)["ai-cli"].(map[string]any)
	for name, v := range map[string]any{"darwin-arm64": nil, "freebsd-amd64": fakeSig("x")} {
		row["platform_signatures"] = map[string]any{name: v}
		if validateAgainst(t, "releases.v1.schema.json", rel) == nil {
			t.Errorf("releases schema accepted platform_signatures.%s = %v", name, v)
		}
	}
}

// validateAgainst validates doc against the committed schema file.
func validateAgainst(t *testing.T, schema string, doc map[string]any) error {
	t.Helper()
	var sch jsonschema.Schema
	if err := sch.UnmarshalJSON(readFile(t, filepath.Join("..", "..", "schemas", schema))); err != nil {
		t.Fatal(err)
	}
	res, err := sch.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	return res.Validate(doc)
}

func TestPlatformSignaturesRefused(t *testing.T) {
	cases := []struct {
		name       string
		sigs       map[string]any
		anchorless bool
		want       string
	}{
		{"unknown platform", map[string]any{"freebsd-amd64": fakeSig("x")}, false, "platform_signatures.freebsd-amd64 is not a platform"},
		{"alg other than ed25519", map[string]any{"darwin-arm64": map[string]any{"key_id": "k", "alg": "rsa", "sig": "s"}}, false, "platform_signatures.darwin-arm64 needs key_id, sig and alg ed25519"},
		{"empty key_id", map[string]any{"darwin-arm64": map[string]any{"key_id": "", "alg": "ed25519", "sig": "s"}}, false, "needs key_id, sig and alg ed25519"},
		{"empty sig", map[string]any{"darwin-arm64": map[string]any{"key_id": "k", "alg": "ed25519", "sig": ""}}, false, "needs key_id, sig and alg ed25519"},
		{"null value", map[string]any{"darwin-arm64": nil}, false, "needs key_id, sig and alg ed25519"},
		{"null source signature", map[string]any{"darwin-arm64": fakeSig("x")}, true, "platform_signatures needs a release_signature"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := signedTree(t, c.sigs)
			if c.anchorless {
				rel := decodeMap(t, readFile(t, filepath.Join(dir, "releases.json")))
				rel["releases"].(map[string]any)["ai-cli"].(map[string]any)["release_signature"] = nil
				b, _ := json.Marshal(rel)
				writeReleases(t, dir, string(b))
			}
			expectProblem(t, c.want, tierArgs("free", dir, t.TempDir(), "")...)
		})
	}

	// a platform with a signature but no checksum
	dir := signedTree(t, map[string]any{"darwin-arm64": fakeSig("x")})
	rel := decodeMap(t, readFile(t, filepath.Join(dir, "releases.json")))
	delete(rel["releases"].(map[string]any)["ai-cli"].(map[string]any)["platform_checksums"].(map[string]any), "darwin-arm64")
	b, _ := json.Marshal(rel)
	writeReleases(t, dir, string(b))
	expectProblem(t, "platform_signatures.darwin-arm64 has no platform_checksums.darwin-arm64", tierArgs("free", dir, t.TempDir(), "")...)
}

// A release without platform_signatures writes no key and the committed goldens
// stay as they are (TestGoldenRegistriesAndCatalog compares the bytes).
func TestPlatformSignaturesAbsentUnchanged(t *testing.T) {
	for _, f := range []string{
		filepath.Join(td, "free", "registry.json"), filepath.Join(td, "licensed", "registry.json"),
		filepath.Join(td, "golden", "catalog.json"),
	} {
		if strings.Contains(string(readFile(t, f)), "platform_signatures") {
			t.Errorf("%s must not carry platform_signatures", f)
		}
	}
	root := t.TempDir()
	out := filepath.Join(root, "free")
	mustGen(t, tierArgs("free", signedTree(t, nil), out, "")...)
	entry := decodeMap(t, readFile(t, filepath.Join(out, "registry.json")))["plugins"].(map[string]any)["ai-cli"].(map[string]any)
	if _, ok := entry["platform_signatures"]; ok {
		t.Fatal("a signed release without platform_signatures must not write the key")
	}
	if entry["release_signature"] == nil {
		t.Fatal("release_signature must still be projected")
	}
}
