package signature

// keys.go — the compiled-in trust root for plugin release signatures.
//
// Purpose: hold the Ed25519 public keys (with ids) and the revoked-id list the CLI trusts for plugin release signatures, one set per tier (hq PLUG D7, contract plugins.release-signature v1).
// Inputs: nothing at runtime; the table is source. No file, environment variable, registry field or manifest field adds a key.
// Outputs: Keys, Revoked, Pinned and the per-tier verifier Verify builds from them.
// Constraints: public keys only. Key ids are derived (plugins-<16 hex of sha256(pub)>), so revoking an id revokes the key. A key verifies only its own tier. The production free key is pinned by P7-PLUG-71 (it equals plugins:scripts/release/keys/plugins-release-free-2ebca8dfad98e19e.pub); the licensed key is pinned by P7-PLUG-72, so the licensed tier stays "unpinned" until then (see Decide). One key per tier, never one key for both. Tests swap the table with signaturetest.UseKeys, never by editing this file.

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"

	"github.com/nself-org/cli/sdk/go/v2/signing"
)

// The two plugin tiers. A key serves exactly one of them.
const (
	TierFree     = "free"
	TierLicensed = "licensed"
)

// Key is one trusted release-signing public key.
type Key struct {
	ID        string // derived: plugins-<first 16 hex of sha256(PublicKey)>
	Tier      string // TierFree or TierLicensed
	PublicKey ed25519.PublicKey
}

// Keys is the trust root. Free: pinned by P7-PLUG-71. Licensed: empty until P7-PLUG-72.
var Keys = []Key{
	{
		ID:        "plugins-2ebca8dfad98e19e",
		Tier:      TierFree,
		PublicKey: mustKey("4cc89067123feba2e4172e4a384b0cb3aa455aba0b2d7e89f1e979070ff1fa75"),
	},
}

// mustKey decodes a 64-hex-character Ed25519 public key. It panics on a bad
// literal; TestFreeTierPinned and TestKeyTable would fail first.
func mustKey(hexKey string) ed25519.PublicKey {
	b, err := hex.DecodeString(hexKey)
	if err != nil || len(b) != ed25519.PublicKeySize {
		panic("signature: malformed pinned public key literal")
	}
	return ed25519.PublicKey(b)
}

// Revoked lists key ids that no longer verify anything, whatever Keys holds.
var Revoked = []string{}

// validTier reports whether tier is one of the two plugin tiers.
func validTier(tier string) bool { return tier == TierFree || tier == TierLicensed }

// Pinned reports whether the key table holds at least one key for tier. A
// revoked key still counts: a tier that once pinned a key never falls back to
// treating signatures as absent.
func Pinned(tier string) bool {
	for _, k := range Keys {
		if k.Tier == tier {
			return true
		}
	}
	return false
}

// verifierFor builds the verifier for one tier over the whole table, so a
// duplicate id, a key listed under two tiers or a malformed key fails closed
// for every tier, and a key of the other tier gives signing.ErrWrongScope.
func verifierFor(tier string) (*signing.Verifier, error) {
	if !validTier(tier) {
		return nil, fmt.Errorf("%w: unknown tier %q", signing.ErrMalformed, tier)
	}
	keys := make([]signing.Key, 0, len(Keys))
	for _, k := range Keys {
		if !validTier(k.Tier) {
			return nil, fmt.Errorf("%w: key %q has unknown tier %.64q", signing.ErrMalformed, k.ID, k.Tier)
		}
		keys = append(keys, signing.Key{ID: k.ID, Purpose: signing.PurposePlugins, Scope: k.Tier, Public: k.PublicKey})
	}
	return signing.NewVerifier(signing.PurposePlugins, keys, Revoked, signing.WithScope(tier))
}

// CheckKeys reports whether the key table is usable: ids derived from the
// keys, no id or public key repeated, no key listed under two tiers, no
// malformed or small-order key. A broken table fails every Verify closed;
// TestKeyTable runs this over the committed table.
func CheckKeys() error {
	for _, tier := range []string{TierFree, TierLicensed} {
		if _, err := verifierFor(tier); err != nil {
			return err
		}
	}
	return nil
}
