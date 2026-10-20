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

// UsePresigning replaces the pre-signing allowlist (tier -> slug -> releases) for the test.
func UsePresigning(t testing.TB, list map[string]map[string][]signature.PresigningRelease) {
	t.Helper()
	old := signature.Presigning
	signature.Presigning = list
	t.Cleanup(func() { signature.Presigning = old })
}

// AllowPresigning adds one release to the current pre-signing allowlist for the test.
func AllowPresigning(t testing.TB, tier, slug, version, sha256hex string) {
	t.Helper()
	next := map[string]map[string][]signature.PresigningRelease{}
	for tr, m := range signature.Presigning {
		next[tr] = map[string][]signature.PresigningRelease{}
		for s, l := range m {
			next[tr][s] = append([]signature.PresigningRelease(nil), l...)
		}
	}
	if next[tier] == nil {
		next[tier] = map[string][]signature.PresigningRelease{}
	}
	next[tier][slug] = append(next[tier][slug], signature.PresigningRelease{Version: version, SHA256: sha256hex})
	UsePresigning(t, next)
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

// SignPlatform returns the platform_signatures entry of the platform signing
// input for slug/version/platform/sha256.
func SignPlatform(t testing.TB, s signing.Signer, slug, version, platform, sha256hex string) *signature.Release {
	t.Helper()
	sig, err := s.Sign(signature.SigningInputPlatform(slug, version, platform, sha256hex))
	if err != nil {
		t.Fatalf("signaturetest: sign platform: %v", err)
	}
	return &signature.Release{KeyID: s.KeyID(), Alg: signature.Alg, Sig: signing.EncodeSig(sig)}
}
