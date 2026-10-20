package signature_test

// signed_since_test.go — the real free release (P7-PLUG-71): the committed floor and one genuine signed entry.
//
// Purpose: prove signed_since.json carries a non-empty free-tier floor generated from the P7-PLUG-69 release data, and that a real signed free entry verifies in-process while a one-byte change to its signature fails.
// Inputs: signed_since.json, testdata/real/ci-1.2.11.json (slug, version, sha256, release_signature copied from plugins releases.json at the release-data sha).
// Outputs: test results only.
// Constraints: no network, no clock. The licensed tier is only checked for presence and emptiness: P7-PLUG-72 fills it and changes that line.

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"

	"github.com/nself-org/cli/internal/plugin/verify/signature"
	"github.com/nself-org/cli/sdk/go/v2/signing"
)

// releaseDataSHA is the plugins release-data commit P7-PLUG-69 recorded as release-data-sha for v1.2.11.
const releaseDataSHA = "63f29729fa637558f7fecf99672fcfb9623c4ba7"

// wholeCatalogueSHA is the release-data commit P7-PLUG-86 recorded for v1.2.13, the whole-catalogue release that carries the first platform signatures.
const wholeCatalogueSHA = "1349341da7c75b91a5eaffe7710d7e2ddd30f6c4"

// TestSignedSinceFreeNonEmpty: the free floor is filled and traces to the evidence sha; the licensed tier is present and still empty.
func TestSignedSinceFreeNonEmpty(t *testing.T) {
	data, err := os.ReadFile("signed_since.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Refs  map[string][]string          `json:"refs"`
		Tiers map[string]map[string]string `json:"tiers"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Tiers[signature.TierFree]) == 0 {
		t.Fatal("free-tier signed-since floor is empty")
	}
	for slug, v := range doc.Tiers[signature.TierFree] {
		if slug == "" || v == "" {
			t.Errorf("free floor has an empty entry %q=%q", slug, v)
		}
	}
	if doc.Tiers[signature.TierFree]["ci"] == "" {
		t.Error("free floor does not name the ci plugin, the first signed free release")
	}
	licensed, ok := doc.Tiers[signature.TierLicensed]
	if !ok || len(licensed) != 0 {
		t.Errorf("licensed tier = %v (present %v): it stays empty until P7-PLUG-72", licensed, ok)
	}
	refForm := regexp.MustCompile(`^nself-org/plugins@[0-9a-f]{40}$`)
	found, foundAll := false, false
	for _, r := range doc.Refs[signature.TierFree] {
		if !refForm.MatchString(r) {
			t.Errorf("free ref %q is not nself-org/plugins@<40 hex>", r)
		}
		if r == "nself-org/plugins@"+releaseDataSHA {
			found = true
		}
		if r == "nself-org/plugins@"+wholeCatalogueSHA {
			foundAll = true
		}
	}
	if !foundAll {
		t.Errorf("free refs %v lack the PLUG-86 whole-catalogue release-data sha %s", doc.Refs[signature.TierFree], wholeCatalogueSHA)
	}
	if n := len(doc.Tiers[signature.TierFree]); n < 100 {
		t.Errorf("free floor names %d slugs, want the whole catalogue (at least 100)", n)
	}
	if !found {
		t.Errorf("free refs %v lack the PLUG-69 release-data sha %s", doc.Refs[signature.TierFree], releaseDataSHA)
	}
	if len(doc.Refs[signature.TierLicensed]) != 0 {
		t.Errorf("licensed refs = %v, want none until P7-PLUG-72", doc.Refs[signature.TierLicensed])
	}
}

// realEntry is the committed copy of one signed registry entry.
type realEntry struct {
	Slug, Version, SHA256 string
	ReleaseSignature      signature.Release `json:"release_signature"`
}

// TestRealRelease: the real ci 1.2.11 entry verifies against the compiled-in free key; one flipped signature byte fails with E121, and the
// free key is no key for the licensed tier (E122).
func TestRealRelease(t *testing.T) {
	data, err := os.ReadFile("testdata/real/ci-1.2.11.json")
	if err != nil {
		t.Fatal(err)
	}
	var e realEntry
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatal(err)
	}
	rel := e.ReleaseSignature
	if err := signature.Verify(signature.TierFree, e.Slug, e.Version, e.SHA256, &rel); err != nil {
		t.Fatalf("real signed entry does not verify: %v", err)
	}
	out, err := signature.Decide(signature.Input{Tier: signature.TierFree, Slug: e.Slug, Version: e.Version, SHA256: e.SHA256, Release: &rel, V15: true})
	if err != nil || !out.Verified {
		t.Fatalf("Decide on the real entry: out=%+v err=%v", out, err)
	}
	sig, err := signing.DecodeSig(rel.Sig)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range []int{0, len(sig) / 2, len(sig) - 1} {
		bad := rel
		flipped := append([]byte(nil), sig...)
		flipped[i] ^= 0x01
		bad.Sig = signing.EncodeSig(flipped)
		if got := code(signature.Verify(signature.TierFree, e.Slug, e.Version, e.SHA256, &bad)); got != "E121" {
			t.Errorf("signature byte %d flipped: code %q, want E121", i, got)
		}
	}
	if got := code(signature.Verify(signature.TierFree, e.Slug, "1.2.12", e.SHA256, &rel)); got != "E121" {
		t.Errorf("other version, same signature: code %q, want E121", got)
	}
	if got := code(signature.Verify(signature.TierLicensed, e.Slug, e.Version, e.SHA256, &rel)); got != "E122" {
		t.Errorf("free key checked for the licensed tier: code %q, want E122", got)
	}
}

// realPlatformEntry is the committed copy of the platform checksums and signatures of one signed binary plugin.
type realPlatformEntry struct {
	Slug, Version string
	Platforms     map[string]struct {
		SHA256            string            `json:"sha256"`
		PlatformSignature signature.Release `json:"platform_signature"`
	}
}

// TestRealPlatformRelease: every platform signature of the real ci 1.2.13 release (the first whole-catalogue release with platform signatures,
// P7-PLUG-86) verifies against the compiled-in free key; a flipped signature byte, another platform's hash and the licensed tier all fail.
func TestRealPlatformRelease(t *testing.T) {
	data, err := os.ReadFile("testdata/real/ci-1.2.13-platforms.json")
	if err != nil {
		t.Fatal(err)
	}
	var e realPlatformEntry
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatal(err)
	}
	if len(e.Platforms) != 5 {
		t.Fatalf("fixture has %d platforms, want 5", len(e.Platforms))
	}
	for plat, p := range e.Platforms {
		rel := p.PlatformSignature
		if err := signature.VerifyPlatform(signature.TierFree, e.Slug, e.Version, plat, p.SHA256, &rel); err != nil {
			t.Errorf("%s: real platform signature does not verify: %v", plat, err)
			continue
		}
		sig, err := signing.DecodeSig(rel.Sig)
		if err != nil {
			t.Fatal(err)
		}
		bad := rel
		flipped := append([]byte(nil), sig...)
		flipped[len(flipped)/2] ^= 0x01
		bad.Sig = signing.EncodeSig(flipped)
		if got := code(signature.VerifyPlatform(signature.TierFree, e.Slug, e.Version, plat, p.SHA256, &bad)); got != "E121" {
			t.Errorf("%s: flipped byte: code %q, want E121", plat, got)
		}
		if got := code(signature.VerifyPlatform(signature.TierLicensed, e.Slug, e.Version, plat, p.SHA256, &rel)); got != "E122" {
			t.Errorf("%s: free key checked for the licensed tier: code %q, want E122", plat, got)
		}
		for other, q := range e.Platforms {
			if other == plat {
				continue
			}
			if got := code(signature.VerifyPlatform(signature.TierFree, e.Slug, e.Version, plat, q.SHA256, &rel)); got != "E121" {
				t.Errorf("%s signature over %s hash: code %q, want E121", plat, other, got)
			}
		}
	}
}
