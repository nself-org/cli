package signature_test

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/plugin/verify/signature"
	"github.com/nself-org/cli/internal/plugin/verify/signature/signaturetest"
	"github.com/nself-org/cli/sdk/go/v2/signing"
)

func TestVerifyGood(t *testing.T) {
	f := newFixture(t)
	sha := digest("tarball")
	if err := signature.Verify(signature.TierFree, slug, version, sha, signaturetest.Sign(t, f.freeSign, slug, version, sha)); err != nil {
		t.Fatalf("free: %v", err)
	}
	if err := signature.Verify(signature.TierLicensed, slug, version, sha, signaturetest.Sign(t, f.licensedSign, slug, version, sha)); err != nil {
		t.Fatalf("licensed: %v", err)
	}
}

func TestVerifyBadSignatureE121(t *testing.T) {
	f := newFixture(t)
	sha := digest("tarball")
	good := signaturetest.Sign(t, f.freeSign, slug, version, sha)
	flipped := []byte(good.Sig)
	if flipped[0] == 'A' {
		flipped[0] = 'B'
	} else {
		flipped[0] = 'A'
	}
	short := signing.EncodeSig(make([]byte, 63))
	cases := map[string]struct {
		slug, version, sha string
		rel                *signature.Release
	}{
		"nil release":        {slug, version, sha, nil},
		"flipped sig byte":   {slug, version, sha, &signature.Release{KeyID: good.KeyID, Alg: good.Alg, Sig: string(flipped)}},
		"other sha":          {slug, version, digest("other"), good},
		"other version":      {slug, "1.2.8", sha, good},
		"other slug":         {"minio", version, sha, good},
		"wrong alg":          {slug, version, sha, &signature.Release{KeyID: good.KeyID, Alg: "rsa", Sig: good.Sig}},
		"empty alg":          {slug, version, sha, &signature.Release{KeyID: good.KeyID, Sig: good.Sig}},
		"not base64":         {slug, version, sha, &signature.Release{KeyID: good.KeyID, Alg: good.Alg, Sig: "!!!"}},
		"short signature":    {slug, version, sha, &signature.Release{KeyID: good.KeyID, Alg: good.Alg, Sig: short}},
		"empty release":      {slug, version, sha, &signature.Release{}},
		"uppercase sha":      {slug, version, strings.ToUpper(sha), good},
		"short sha":          {slug, version, sha[:63], good},
		"non-hex sha":        {slug, version, strings.Repeat("g", 64), good},
		"newline in slug":    {"ci\nx", version, sha, good},
		"newline in version": {slug, "1.2.7\n", sha, good},
		"carriage return":    {slug, "1.2.7\r", sha, good},
		"nul in slug":        {"ci\x00", version, sha, good},
		"empty slug":         {"", version, sha, good},
		"empty version":      {slug, "", sha, good},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := signature.Verify(signature.TierFree, c.slug, c.version, c.sha, c.rel)
			if code(err) != "E121" {
				t.Fatalf("want E121, got %v", err)
			}
		})
	}
	t.Run("unknown tier fails closed", func(t *testing.T) {
		if code(signature.Verify("enterprise", slug, version, sha, good)) != "E121" {
			t.Fatal("unknown tier must be E121")
		}
	})
	t.Run("broken key table fails closed", func(t *testing.T) {
		signaturetest.UseKeys(t, []signature.Key{f.free, f.free}, nil)
		if code(signature.Verify(signature.TierFree, slug, version, sha, good)) != "E121" {
			t.Fatal("duplicate key must fail closed with E121")
		}
	})
}

func TestVerifyUnknownKeyE122(t *testing.T) {
	f := newFixture(t)
	sha := digest("tarball")
	_, stranger := signaturetest.NewKey(t, signature.TierFree)
	rel := signaturetest.Sign(t, stranger, slug, version, sha)
	err := signature.Verify(signature.TierFree, slug, version, sha, rel)
	if code(err) != "E122" {
		t.Fatalf("unknown key: want E122, got %v", err)
	}
	if !strings.Contains(err.Error(), rel.KeyID) {
		t.Errorf("message must name the key id: %v", err)
	}
	// An empty table (the unpinned state) knows no key either.
	signaturetest.UseKeys(t, nil, nil)
	if code(signature.Verify(signature.TierFree, slug, version, sha, signaturetest.Sign(t, f.freeSign, slug, version, sha))) != "E122" {
		t.Fatal("empty table: want E122")
	}
	// A key id with characters outside the id alphabet is malformed, not unknown.
	signaturetest.UseKeys(t, []signature.Key{f.free}, nil)
	bad := signaturetest.Sign(t, f.freeSign, slug, version, sha)
	bad.KeyID = "plugins-\n"
	if code(signature.Verify(signature.TierFree, slug, version, sha, bad)) != "E121" {
		t.Fatal("malformed key id: want E121")
	}
}

func TestVerifyRevokedKeyE123(t *testing.T) {
	f := newFixture(t)
	sha := digest("tarball")
	rel := signaturetest.Sign(t, f.freeSign, slug, version, sha)
	signaturetest.UseKeys(t, []signature.Key{f.free, f.licensed}, []string{f.free.ID})
	err := signature.Verify(signature.TierFree, slug, version, sha, rel)
	if code(err) != "E123" {
		t.Fatalf("revoked: want E123, got %v", err)
	}
	// Revocation wins over a bad signature: the key is not consulted.
	rel.Sig = signing.EncodeSig(make([]byte, 64))
	if code(signature.Verify(signature.TierFree, slug, version, sha, rel)) != "E123" {
		t.Fatal("revoked key with a bad signature must still be E123")
	}
	// A revoked key does not unpin its tier.
	if !signature.Pinned(signature.TierFree) {
		t.Fatal("a revoked key must not unpin the tier")
	}
}

func TestVerifyTierIsolation(t *testing.T) {
	f := newFixture(t)
	sha := digest("tarball")
	freeSig := signaturetest.Sign(t, f.freeSign, slug, version, sha)
	licSig := signaturetest.Sign(t, f.licensedSign, slug, version, sha)
	if err := signature.Verify(signature.TierFree, slug, version, sha, freeSig); err != nil {
		t.Fatalf("free fixture with the free key: %v", err)
	}
	if err := signature.Verify(signature.TierLicensed, slug, version, sha, licSig); err != nil {
		t.Fatalf("licensed fixture with the licensed key: %v", err)
	}
	if code(signature.Verify(signature.TierLicensed, slug, version, sha, freeSig)) != "E122" {
		t.Fatal("a free-key signature on a licensed entry must be E122")
	}
	if code(signature.Verify(signature.TierFree, slug, version, sha, licSig)) != "E122" {
		t.Fatal("a licensed-key signature on a free entry must be E122")
	}
}

func TestSigningInput(t *testing.T) {
	got := string(signature.SigningInput("ci", "1.2.7", "ab"))
	if got != "nself-plugin-v1\nci\n1.2.7\nab\n" {
		t.Fatalf("signing input = %q", got)
	}
}

func TestKeyTable(t *testing.T) {
	if err := signature.CheckKeys(); err != nil {
		t.Fatalf("committed key table: %v", err)
	}
	if signature.Pinned(signature.TierFree) || signature.Pinned(signature.TierLicensed) {
		t.Log("a production key is pinned; P7-PLUG-71/72 own the both-tier non-empty tests")
	}
	f := newFixture(t)
	if err := signature.CheckKeys(); err != nil {
		t.Fatalf("fixture table: %v", err)
	}
	other, _ := signaturetest.NewKey(t, signature.TierFree)
	notDerived := other
	notDerived.ID = "plugins-0000000000000000"
	sameKeyOtherTier := f.free
	sameKeyOtherTier.Tier = signature.TierLicensed
	repeatedID := other
	repeatedID.ID = f.free.ID
	for name, table := range map[string][]signature.Key{
		"repeated id":                 {f.free, repeatedID},
		"repeated key":                {f.free, f.free},
		"one key serving both tiers":  {f.free, sameKeyOtherTier},
		"id not derived from the key": {notDerived},
		"unknown tier":                {{ID: other.ID, Tier: "enterprise", PublicKey: other.PublicKey}},
		"short public key":            {{ID: other.ID, Tier: signature.TierFree, PublicKey: other.PublicKey[:31]}},
		"small-order (identity) key":  {{ID: signing.KeyID(signing.PurposePlugins, identityKey()), Tier: signature.TierFree, PublicKey: identityKey()}},
	} {
		t.Run(name, func(t *testing.T) {
			signaturetest.UseKeys(t, table, nil)
			if signature.CheckKeys() == nil {
				t.Fatal("CheckKeys accepted a broken table")
			}
		})
	}
}

// identityKey is the encoding of the identity point, a small-order key.
func identityKey() []byte {
	k := make([]byte, 32)
	k[0] = 1
	return k
}
