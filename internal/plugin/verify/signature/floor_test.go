package signature_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/nself-org/cli/internal/plugin/verify/signature"
	"github.com/nself-org/cli/internal/plugin/verify/signature/signaturetest"
)

// TestFloorRule: the fixture floor is swapped in with the helper, never the
// committed map. At or above the slug's floor, or for a slug the floor does
// not name, a missing signature is E107 in BOTH modes; below the floor the
// ordinary v1.4 warning applies; an empty tier floor applies no rule.
func TestFloorRule(t *testing.T) {
	signaturetest.UseKeys(t, nil, nil)
	signaturetest.UseFloor(t, map[string]map[string]string{
		signature.TierFree:     {"ci": "1.2.7", "pre": "2.0.0", "p": "v1.0.0"},
		signature.TierLicensed: {},
	})
	at := func(tier, s, v string, v15 bool) error {
		_, err := signature.Decide(signature.Input{Tier: tier, Slug: s, Version: v, SHA256: digest("x"), V15: v15})
		return err
	}
	for _, v15 := range []bool{false, true} {
		for _, c := range []struct {
			slug, ver string
			refused   bool
		}{
			{"ci", "1.2.7", true},        // at the floor
			{"ci", "1.2.8", true},        // above, patch
			{"ci", "1.3.0", true},        // above, minor
			{"ci", "2.0.0", true},        // above, major
			{"ci", "10.0.0", true},       // numeric, not lexical
			{"ci", "1.2.6", v15},         // below: v1.4 warns, v1.5 refuses as plain missing
			{"ci", "1.1.9", v15},         // below, minor
			{"ci", "0.9.9", v15},         // below, major
			{"ci", "1.2.7+build5", true}, // build metadata ignored
			{"ci", "v1.2.7", true},       // leading v
			{"ci", "garbage", true},      // unparseable: fail closed
			{"ci", "1.2", true},          // not MAJOR.MINOR.PATCH
			{"ci", "1.2.x", true},        // non-numeric part
			{"ci", "1.2.-3", true},       // negative part
			{"newslug", "0.0.1", true},   // slug absent from the floor: first released after signing began
			{"pre", "2.0.0-rc.1", v15},   // a pre-release sorts below its release
			{"pre", "2.0.0", true},
			{"p", "1.0.0", true}, // floor written with a leading v
		} {
			err := at(signature.TierFree, c.slug, c.ver, v15)
			if (err != nil) != c.refused || (err != nil && code(err) != "E107") {
				t.Errorf("free %s %s v15=%v: err=%v, want refused=%v", c.slug, c.ver, v15, err, c.refused)
			}
		}
		// Empty tier floor: no floor rule, even for a slug the (other tier's) floor names.
		if err := at(signature.TierLicensed, "ci", "9.9.9", v15); (err != nil) != v15 {
			t.Errorf("licensed (empty floor) v15=%v: err=%v", v15, err)
		}
	}
	// A pre-release floor: its own pre-release is at the floor, a lower pre-release is below.
	signaturetest.UseFloor(t, map[string]map[string]string{signature.TierFree: {"ci": "1.0.0-rc.2"}})
	for _, c := range []struct {
		ver     string
		refused bool
	}{{"1.0.0-rc.2", true}, {"1.0.0-rc.3", true}, {"1.0.0-rc.1", false}, {"1.0.0", true}, {"0.9.9", false}} {
		if err := at(signature.TierFree, "ci", c.ver, false); (err != nil) != c.refused {
			t.Errorf("pre-release floor, version %s: err=%v want refused=%v", c.ver, err, c.refused)
		}
	}
	// An unparseable floor value fails closed.
	signaturetest.UseFloor(t, map[string]map[string]string{signature.TierFree: {"ci": "weird"}})
	if err := at(signature.TierFree, "ci", "1.0.0", false); err == nil {
		t.Error("unparseable floor value must require a signature")
	}
	// With a signature present and a pinned key the floor is satisfied.
	key, signer := signaturetest.NewKey(t, signature.TierFree)
	signaturetest.UseKeys(t, []signature.Key{key}, nil)
	signaturetest.UseFloor(t, map[string]map[string]string{signature.TierFree: {"ci": "1.2.7"}})
	sha := digest("x")
	out, err := signature.Decide(signature.Input{Tier: signature.TierFree, Slug: "ci", Version: "1.2.7", SHA256: sha, Release: signaturetest.Sign(t, signer, "ci", "1.2.7", sha)})
	if err != nil || !out.Verified {
		t.Fatalf("signed entry at the floor: out=%+v err=%v", out, err)
	}
}

// TestCommittedFloor: signed_since.json parses, carries both tiers and, until
// P7-PLUG-71/72 fill it, is empty. Pinned tiers must not stay empty then.
func TestCommittedFloor(t *testing.T) {
	data, err := os.ReadFile("signed_since.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Tiers map[string]map[string]string `json:"tiers"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	for _, tier := range []string{signature.TierFree, signature.TierLicensed} {
		m, ok := doc.Tiers[tier]
		if !ok {
			t.Fatalf("signed_since.json lacks tier %q", tier)
		}
		if signature.Pinned(tier) && len(m) == 0 {
			t.Errorf("tier %q is pinned but its signed-since floor is empty", tier)
		}
	}
	if len(signature.Floor) != len(doc.Tiers) {
		t.Errorf("embedded floor has %d tiers, file has %d", len(signature.Floor), len(doc.Tiers))
	}
}
