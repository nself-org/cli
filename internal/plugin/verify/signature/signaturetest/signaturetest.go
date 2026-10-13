// Package signaturetest gives tests a throwaway release key and swaps the
// compiled-in key table and floor, restoring both on t.Cleanup. Keys are
// generated per call and never written anywhere; no production key belongs
// here, and nothing outside tests imports this package.
package signaturetest

import (
	"testing"

	"github.com/nself-org/cli/internal/plugin/verify/signature"
	"github.com/nself-org/cli/sdk/go/v2/signing"
	"github.com/nself-org/cli/sdk/go/v2/signing/signingtest"
)

// UseKeys replaces the trusted key table and revoked list for the test.
func UseKeys(t testing.TB, keys []signature.Key, revoked []string) {
	t.Helper()
	oldKeys, oldRevoked := signature.Keys, signature.Revoked
	signature.Keys, signature.Revoked = keys, revoked
	t.Cleanup(func() { signature.Keys, signature.Revoked = oldKeys, oldRevoked })
}

// UseFloor replaces the signed-since floor (tier -> slug -> version) for the test.
func UseFloor(t testing.TB, floor map[string]map[string]string) {
	t.Helper()
	old := signature.Floor
	signature.Floor = floor
	t.Cleanup(func() { signature.Floor = old })
}

// NewKey generates a key for tier with its derived id, and a signer for it.
func NewKey(t testing.TB, tier string) (signature.Key, signing.Signer) {
	t.Helper()
	k, s := signingtest.NewKey(t, signing.PurposePlugins)
	return signature.Key{ID: k.ID, Tier: tier, PublicKey: k.Public}, s
}

// Sign returns the release_signature of the signing input for slug/version/sha256.
func Sign(t testing.TB, s signing.Signer, slug, version, sha256hex string) *signature.Release {
	t.Helper()
	sig, err := s.Sign(signature.SigningInput(slug, version, sha256hex))
	if err != nil {
		t.Fatalf("signaturetest: sign: %v", err)
	}
	return &signature.Release{KeyID: s.KeyID(), Alg: signature.Alg, Sig: signing.EncodeSig(sig)}
}
