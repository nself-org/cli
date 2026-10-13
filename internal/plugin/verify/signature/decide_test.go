package signature_test

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/plugin/verify/signature"
	"github.com/nself-org/cli/internal/plugin/verify/signature/signaturetest"
)

// TestUnpinnedTier: with no key for a tier a signature is unverifiable and
// counts as missing (v1.4 warns, v1.5 E107); with a generated key swapped in
// the same entry verifies, and an unknown or revoked key id gives E122/E123.
// It swaps the table with the helper and never reads the committed one.
func TestUnpinnedTier(t *testing.T) {
	sha := digest("tarball")
	signaturetest.UseKeys(t, nil, nil)
	signaturetest.UseFloor(t, map[string]map[string]string{})
	key, signer := signaturetest.NewKey(t, signature.TierFree)
	rel := signaturetest.Sign(t, signer, slug, version, sha)
	in := signature.Input{Tier: signature.TierFree, Slug: slug, Version: version, SHA256: sha, Release: rel}

	out, err := signature.Decide(in)
	if err != nil || out.Verified || !strings.Contains(out.Warning, "no trusted key") {
		t.Fatalf("v1.4 unpinned: out=%+v err=%v", out, err)
	}
	in.V15 = true
	if _, err := signature.Decide(in); code(err) != "E107" {
		t.Fatalf("v1.5 unpinned: want E107, got %v", err)
	}
	in.V15 = false
	in.Strict = true
	if _, err := signature.Decide(in); code(err) != "E107" {
		t.Fatalf("strict unpinned: want E107, got %v", err)
	}

	// Pin a generated key for the tier: the same entry now verifies.
	signaturetest.UseKeys(t, []signature.Key{key}, nil)
	for _, v15 := range []bool{false, true} {
		in := signature.Input{Tier: signature.TierFree, Slug: slug, Version: version, SHA256: sha, Release: rel, V15: v15}
		out, err := signature.Decide(in)
		if err != nil || !out.Verified || out.Warning != "" {
			t.Fatalf("pinned (v15=%v): out=%+v err=%v", v15, out, err)
		}
	}
	// A pinned tier never falls back: unknown and revoked keys refuse in v1.4 too.
	_, stranger := signaturetest.NewKey(t, signature.TierFree)
	unknown := signaturetest.Sign(t, stranger, slug, version, sha)
	if _, err := signature.Decide(signature.Input{Tier: signature.TierFree, Slug: slug, Version: version, SHA256: sha, Release: unknown}); code(err) != "E122" {
		t.Fatalf("unknown key in v1.4: want E122, got %v", err)
	}
	signaturetest.UseKeys(t, []signature.Key{key}, []string{key.ID})
	if _, err := signature.Decide(signature.Input{Tier: signature.TierFree, Slug: slug, Version: version, SHA256: sha, Release: rel}); code(err) != "E123" {
		t.Fatalf("revoked key in v1.4: want E123, got %v", err)
	}
	// Pinning the free tier does not pin the licensed tier.
	signaturetest.UseKeys(t, []signature.Key{key}, nil)
	if signature.Pinned(signature.TierLicensed) {
		t.Fatal("licensed tier must stay unpinned")
	}
}

// TestDecideMissing: the missing-signature matrix.
func TestDecideMissing(t *testing.T) {
	signaturetest.UseKeys(t, nil, nil)
	signaturetest.UseFloor(t, map[string]map[string]string{})
	base := signature.Input{Tier: signature.TierFree, Slug: slug, Version: version, SHA256: digest("x")}

	out, err := signature.Decide(base)
	if err != nil || out.Warning == "" || out.Verified {
		t.Fatalf("v1.4 missing: out=%+v err=%v", out, err)
	}
	v15 := base
	v15.V15 = true
	if _, err := signature.Decide(v15); code(err) != "E107" {
		t.Fatalf("v1.5 missing: want E107, got %v", err)
	}
	strict := base
	strict.Strict = true
	if _, err := signature.Decide(strict); code(err) != "E107" {
		t.Fatalf("strict missing: want E107, got %v", err)
	}
	allow := v15
	allow.AllowUnsigned = true
	out, err = signature.Decide(allow)
	if err != nil || !strings.Contains(out.Warning, "--allow-unsigned") {
		t.Fatalf("allow-unsigned (local): out=%+v err=%v", out, err)
	}
}
