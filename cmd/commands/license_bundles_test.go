package commands

import (
	"context"
	"reflect"
	"testing"

	"github.com/nself-org/cli/internal/license"
)

// TestResolveBundlesUnlocked_PlusUnion is a regression test for
// P6-E6-W4-S3-T7 (2026-09-03): `nself license show --json`'s bundles_unlocked
// field for an ɳSelf+ key previously always resolved to []string{"ɳSelf+"}
// (license_status.go:158, pre-fix), never the union of paid bundles.
// bundle.LoadBytes is seeded once for the whole package in bundle_test.go's
// TestMain with fixtureBundlesJSON (6 paid bundles, tv included).
func TestResolveBundlesUnlocked_PlusUnion(t *testing.T) {
	pp := &license.ProductPrefix{Prefix: "nself_plus_", Product: "plus", DisplayName: "ɳSelf+"}
	got := resolveBundlesUnlocked(context.Background(), pp)
	want := []string{"ɳChat", "ɳClaw", "ɳFamily", "ɳSentry", "Media Bundle", "ClawDE"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("plus key bundles_unlocked = %v, want %v (union of every paid bundle, DisplayOrder)", got, want)
	}
}

// TestResolveBundlesUnlocked_OwnerUnion verifies the owner product code
// (all-access, per PPI's NSELF_PLUGIN_LICENSE_KEY_OWNER) resolves the same
// full paid-bundle union as plus.
func TestResolveBundlesUnlocked_OwnerUnion(t *testing.T) {
	pp := &license.ProductPrefix{Prefix: "nself_owner_", Product: "owner", DisplayName: "ɳSelf Owner"}
	got := resolveBundlesUnlocked(context.Background(), pp)
	want := []string{"ɳChat", "ɳClaw", "ɳFamily", "ɳSentry", "Media Bundle", "ClawDE"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("owner key bundles_unlocked = %v, want %v", got, want)
	}
}

// TestResolveBundlesUnlocked_SingleBundle verifies a key tied to exactly one
// bundle (nself_chat_ prefix) resolves to that one bundle only, not the
// full union and not a raw product-prefix name mismatched from
// bundles.json's display string.
func TestResolveBundlesUnlocked_SingleBundle(t *testing.T) {
	cases := []struct {
		product string
		want    string
	}{
		{"chat", "ɳChat"},
		{"claw", "ɳClaw"},
		{"clawde", "ClawDE"},
		{"family", "ɳFamily"},
		{"media", "Media Bundle"}, // nself_media_ prefix predates the "tv" bundles.json slug
	}
	for _, c := range cases {
		pp := &license.ProductPrefix{Prefix: "nself_" + c.product + "_", Product: c.product, DisplayName: "irrelevant"}
		got := resolveBundlesUnlocked(context.Background(), pp)
		want := []string{c.want}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("product %q bundles_unlocked = %v, want %v", c.product, got, want)
		}
	}
}

// TestResolveBundlesUnlocked_UnmappedProductDegradesToDisplayName verifies a
// product code with no bundles.json counterpart (legacy business tiers)
// degrades honestly to the raw product display name rather than fabricating
// a bundle list.
func TestResolveBundlesUnlocked_UnmappedProductDegradesToDisplayName(t *testing.T) {
	pp := &license.ProductPrefix{Prefix: "nself_pro_", Product: "pro", DisplayName: "ɳSelf Pro"}
	got := resolveBundlesUnlocked(context.Background(), pp)
	want := []string{"ɳSelf Pro"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unmapped product bundles_unlocked = %v, want %v (degrade to display name)", got, want)
	}
}

// TestResolveBundlesUnlocked_NilProduct verifies a nil ProductPrefix (key
// prefix not recognized at all) returns nil rather than panicking.
func TestResolveBundlesUnlocked_NilProduct(t *testing.T) {
	got := resolveBundlesUnlocked(context.Background(), nil)
	if got != nil {
		t.Fatalf("nil product bundles_unlocked = %v, want nil", got)
	}
}
