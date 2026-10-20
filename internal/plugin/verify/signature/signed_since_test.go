package signature_test

// signed_since_test.go — the committed floors of both tiers and one genuine signed entry per tier (P7-PLUG-71 free, P7-PLUG-72 licensed).
//
// Purpose: prove signed_since.json carries a non-empty floor for each tier generated from the recorded release data, and that a real signed entry of each tier verifies in-process while a one-byte change to its signature fails.
// Inputs: signed_since.json, testdata/real/ci-1.2.11.json and testdata/real/cron-1.1.2.json (slug, version, sha256, release_signature copied from the plugins and bundles releases.json at the release-data sha).
// Outputs: test results only.
// Constraints: no network, no clock.

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/plugin/verify/signature"
	"github.com/nself-org/cli/sdk/go/v2/signing"
)

// releaseDataSHA is the plugins release-data commit P7-PLUG-69 recorded as release-data-sha for v1.2.11.
const releaseDataSHA = "63f29729fa637558f7fecf99672fcfb9623c4ba7"

// wholeCatalogueSHA is the release-data commit P7-PLUG-86 recorded for v1.2.13, the whole-catalogue release that carries the first platform signatures.
const wholeCatalogueSHA = "1349341da7c75b91a5eaffe7710d7e2ddd30f6c4"

// earlierFreeSHAs are the release-data commits PLUG-69 recorded for the earlier free releases (v1.2.7 and v1.2.9), which still set floors.
var earlierFreeSHAs = []string{"bc6ee963bd70a46c60ff713299d35ab093f8c258", "93cad14e1384f0270f61ec9d28d81043dc86008a"}

// licensedReleaseDataSHA is the bundles release-data commit P7-PLUG-70 recorded as release-data-sha for v1.2.10, the first signed licensed release.
const licensedReleaseDataSHA = "e83373ec5828276dfc6c9b4fe8d9fd08317f62f7"

// signedSinceDoc is the part of signed_since.json the floor tests read.
type signedSinceDoc struct {
	Refs       map[string][]string          `json:"refs"`
	Tiers      map[string]map[string]string `json:"tiers"`
	Presigning map[string]json.RawMessage   `json:"presigning"`
}

// checkFloors reports why doc is not a complete two-tier floor: a missing or empty tier, an empty entry, a recorded ref that is not <repo>@<40 hex>, or a ref list
// that lacks one of the evidence shas (or holds a sha the evidence never recorded).
func checkFloors(doc signedSinceDoc) error {
	// min is a floor on the slug count, not the exact count (the first signed releases name 131 free and 46 licensed slugs); a floor that shrinks below it lost a tier's data.
	want := map[string]struct {
		repo string
		shas []string
		min  int
		must []string
	}{
		signature.TierFree:     {"nself-org/plugins", append([]string{releaseDataSHA, wholeCatalogueSHA}, earlierFreeSHAs...), 100, []string{"ci"}},
		signature.TierLicensed: {"nself-org/bundles", []string{licensedReleaseDataSHA}, 40, []string{"cron", "notify", "claw"}},
	}
	for tier, w := range want {
		// The first signed release of each tier signed every row, so no unsigned pre-signing release may be listed: a row here would let an unsigned release install.
		if got := string(doc.Presigning[tier]); got != "{}" && got != "" {
			return fmt.Errorf("%s pre-signing list is %s, want {} (every release of the first signed release is signed)", tier, got)
		}
		floor, ok := doc.Tiers[tier]
		if !ok || len(floor) == 0 {
			return fmt.Errorf("%s-tier signed-since floor is missing or empty", tier)
		}
		if len(floor) < w.min {
			return fmt.Errorf("%s floor names %d slugs, want the whole catalogue (at least %d)", tier, len(floor), w.min)
		}
		for slug, v := range floor {
			if slug == "" || v == "" {
				return fmt.Errorf("%s floor has an empty entry %q=%q", tier, slug, v)
			}
		}
		for _, slug := range w.must {
			if floor[slug] == "" {
				return fmt.Errorf("%s floor does not name %s", tier, slug)
			}
		}
		refForm := regexp.MustCompile("^" + regexp.QuoteMeta(w.repo) + "@[0-9a-f]{40}$")
		for _, r := range doc.Refs[tier] {
			if !refForm.MatchString(r) {
				return fmt.Errorf("%s ref %q is not %s@<40 hex>", tier, r, w.repo)
			}
			if !slices.Contains(w.shas, r[len(w.repo)+1:]) {
				return fmt.Errorf("%s ref %q is not a release-data sha the evidence recorded", tier, r)
			}
		}
		for _, sha := range w.shas {
			if !slices.Contains(doc.Refs[tier], w.repo+"@"+sha) {
				return fmt.Errorf("%s refs %v lack the evidence release-data sha %s", tier, doc.Refs[tier], sha)
			}
		}
	}
	return nil
}

// TestSignedSinceNonEmpty: both floors are filled and trace to the evidence shas; checkFloors rejects an empty or missing tier, a floor that lost a named slug and a
// ref that is not an evidence sha.
func TestSignedSinceNonEmpty(t *testing.T) {
	data, err := os.ReadFile("signed_since.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc signedSinceDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if err := checkFloors(doc); err != nil {
		t.Fatal(err)
	}
	clone := func(mut func(d *signedSinceDoc)) signedSinceDoc {
		var c signedSinceDoc
		if err := json.Unmarshal(data, &c); err != nil {
			t.Fatal(err)
		}
		mut(&c)
		return c
	}
	for name, mut := range map[string]func(d *signedSinceDoc){
		"licensed tier empty":   func(d *signedSinceDoc) { d.Tiers[signature.TierLicensed] = map[string]string{} },
		"licensed tier missing": func(d *signedSinceDoc) { delete(d.Tiers, signature.TierLicensed) },
		"free tier empty":       func(d *signedSinceDoc) { d.Tiers[signature.TierFree] = map[string]string{} },
		"free tier missing":     func(d *signedSinceDoc) { delete(d.Tiers, signature.TierFree) },
		"empty version":         func(d *signedSinceDoc) { d.Tiers[signature.TierLicensed]["chat"] = "" },
		"cron dropped":          func(d *signedSinceDoc) { delete(d.Tiers[signature.TierLicensed], "cron") },
		"licensed ref not evidence sha": func(d *signedSinceDoc) {
			d.Refs[signature.TierLicensed] = []string{"nself-org/bundles@" + releaseDataSHA}
		},
		"licensed presigning row": func(d *signedSinceDoc) {
			d.Presigning[signature.TierLicensed] = json.RawMessage(`{"cron":[{"version":"1.1.1","sha256":"` + strings.Repeat("a", 64) + `"}]}`)
		},
		"free presigning row": func(d *signedSinceDoc) {
			d.Presigning[signature.TierFree] = json.RawMessage(`{"ci":[{"version":"1.2.1","sha256":"` + strings.Repeat("a", 64) + `"}]}`)
		},
		"licensed ref lost": func(d *signedSinceDoc) { d.Refs[signature.TierLicensed] = nil },
		"free ref wrong repo": func(d *signedSinceDoc) {
			d.Refs[signature.TierFree] = append(d.Refs[signature.TierFree], "nself-org/bundles@"+licensedReleaseDataSHA)
		},
	} {
		if checkFloors(clone(mut)) == nil {
			t.Errorf("%s: checkFloors accepted it", name)
		}
	}
}

// realEntry is the committed copy of one signed registry entry.
type realEntry struct {
	Slug, Version, SHA256 string
	ReleaseSignature      signature.Release `json:"release_signature"`
}

// TestRealRelease: the real ci 1.2.11 entry (free) and the real cron 1.1.2 entry (licensed, bundles v1.2.10) verify against their compiled-in key and decide as
// verified in v1.5 mode; one flipped signature byte fails with E121, another version fails with E121, and the other tier's key is no key for the entry (E122).
func TestRealRelease(t *testing.T) {
	for _, c := range []struct{ file, tier, other string }{
		{"testdata/real/ci-1.2.11.json", signature.TierFree, signature.TierLicensed},
		{"testdata/real/cron-1.1.2.json", signature.TierLicensed, signature.TierFree},
	} {
		data, err := os.ReadFile(c.file)
		if err != nil {
			t.Fatal(err)
		}
		var e realEntry
		if err := json.Unmarshal(data, &e); err != nil {
			t.Fatal(err)
		}
		rel := e.ReleaseSignature
		if err := signature.Verify(c.tier, e.Slug, e.Version, e.SHA256, &rel); err != nil {
			t.Fatalf("%s: real signed entry does not verify: %v", c.tier, err)
		}
		out, err := signature.Decide(signature.Input{Tier: c.tier, Slug: e.Slug, Version: e.Version, SHA256: e.SHA256, Release: &rel, V15: true})
		if err != nil || !out.Verified {
			t.Fatalf("%s: Decide on the real entry: out=%+v err=%v", c.tier, out, err)
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
			if got := code(signature.Verify(c.tier, e.Slug, e.Version, e.SHA256, &bad)); got != "E121" {
				t.Errorf("%s: signature byte %d flipped: code %q, want E121", c.tier, i, got)
			}
		}
		if got := code(signature.Verify(c.tier, e.Slug, "9.9.9", e.SHA256, &rel)); got != "E121" {
			t.Errorf("%s: other version, same signature: code %q, want E121", c.tier, got)
		}
		if got := code(signature.Verify(c.other, e.Slug, e.Version, e.SHA256, &rel)); got != "E122" {
			t.Errorf("%s key checked for the %s tier: code %q, want E122", c.tier, c.other, got)
		}
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

// TestLicensedPairEnforced: cron and notify exist in both tiers and both tiers are pinned (P7-PLUG-72), so each tier's entry is judged by its own tier alone. The real
// signed licensed cron verifies; an unsigned or free-signed licensed cron, notify or chat is refused (E107 for no signature, E122 for the wrong tier's key) in v1.4 mode too,
// because the licensed floor names every slug of the first signed release and the pre-signing list is empty.
func TestLicensedPairEnforced(t *testing.T) {
	data, err := os.ReadFile("testdata/real/cron-1.1.2.json")
	if err != nil {
		t.Fatal(err)
	}
	var e realEntry
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatal(err)
	}
	for _, v15 := range []bool{false, true} {
		rel := e.ReleaseSignature
		out, err := signature.Decide(signature.Input{Tier: signature.TierLicensed, Slug: "cron", Version: e.Version, SHA256: e.SHA256, Release: &rel, V15: v15})
		if err != nil || !out.Verified || out.Warning != "" {
			t.Errorf("licensed cron signed (v15=%v): out=%+v err=%v, want verified", v15, out, err)
		}
		for _, slug := range []string{"cron", "notify", "chat"} {
			if _, err := signature.Decide(signature.Input{Tier: signature.TierLicensed, Slug: slug, Version: "1.1.2", SHA256: "00", V15: v15}); code(err) != "E107" {
				t.Errorf("licensed %s unsigned (v15=%v): code %q, want E107", slug, v15, code(err))
			}
		}
		if _, err := signature.Decide(signature.Input{Tier: signature.TierLicensed, Slug: "claw", Version: "1.1.2", SHA256: "00", V15: v15}); code(err) != "E107" {
			t.Errorf("licensed claw 1.1.2 unsigned, below its floor 1.1.12 (v15=%v): code %q, want E107", v15, code(err))
		}
		if _, err := signature.Decide(signature.Input{Tier: signature.TierFree, Slug: "cron", Version: e.Version, SHA256: e.SHA256, Release: &rel, V15: v15}); code(err) != "E122" {
			t.Errorf("licensed signature on the free cron (v15=%v): code %q, want E122", v15, code(err))
		}
	}
}
