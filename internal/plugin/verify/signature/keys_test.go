package signature_test

// keys_test.go — the pinned production free-tier release key (P7-PLUG-71).
//
// Purpose: prove the compiled-in free key is exactly the public key file the first signed free release (P7-PLUG-69) committed, and that the table cannot silently lose it, double it or share it with the licensed tier.
// Inputs: testdata/real/plugins-release-free-2ebca8dfad98e19e.pub, a byte copy of plugins:scripts/release/keys/ at the P7-PLUG-69 release.
// Outputs: test results only.
// Constraints: public keys only. The licensed tier is pinned by P7-PLUG-72, so nothing here asserts on the licensed table beyond "no key serves two tiers".

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"slices"
	"testing"

	"github.com/nself-org/cli/internal/plugin/verify/signature"
	"github.com/nself-org/cli/internal/plugin/verify/signature/signaturetest"
	"github.com/nself-org/cli/sdk/go/v2/signing"
)

const (
	freeKeyFile   = "testdata/real/plugins-release-free-2ebca8dfad98e19e.pub"
	freeKeyID     = "plugins-2ebca8dfad98e19e"                                         // PLUG-69 evidence key-id
	freeKeyFileSH = "6720a63381e247173ed79f9cdf49e82df0e3f2ed4e7fa7a897b5bde6c6e84a4a" // PLUG-69 evidence key-sha256 (of the file)
)

// readFreeKeyFile returns the raw Ed25519 key and the sha256 of the committed key file.
func readFreeKeyFile(t *testing.T) (ed25519.PublicKey, string) {
	t.Helper()
	raw, err := os.ReadFile(freeKeyFile)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "PUBLIC KEY" {
		t.Fatalf("%s is not a PEM PUBLIC KEY", freeKeyFile)
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	ed, ok := pub.(ed25519.PublicKey)
	if !ok {
		t.Fatalf("%s holds a %T, want an Ed25519 key", freeKeyFile, pub)
	}
	return ed, hex.EncodeToString(sum[:])
}

// checkFreePin reports why table is not a correct free-tier pin for want: exactly one free key equal to want, its id derived from it, no
// public key listed under two tiers, none of the keys revoked.
func checkFreePin(table []signature.Key, revoked []string, want ed25519.PublicKey) error {
	var free []signature.Key
	tiers := map[string]map[string]bool{} // raw key -> tiers it serves
	for _, k := range table {
		if tiers[string(k.PublicKey)] == nil {
			tiers[string(k.PublicKey)] = map[string]bool{}
		}
		tiers[string(k.PublicKey)][k.Tier] = true
		if k.Tier == signature.TierFree {
			free = append(free, k)
		}
		if slices.Contains(revoked, k.ID) {
			return fmt.Errorf("key %s is revoked", k.ID)
		}
	}
	for _, ts := range tiers {
		if len(ts) > 1 {
			return errors.New("one public key serves both tiers")
		}
	}
	if len(free) != 1 {
		return fmt.Errorf("want exactly 1 free key, have %d", len(free))
	}
	k := free[0]
	if !bytes.Equal(k.PublicKey, want) {
		return errors.New("free key differs from the committed key file")
	}
	if k.ID != signing.KeyID(signing.PurposePlugins, k.PublicKey) || k.ID != freeKeyID {
		return fmt.Errorf("free key id %q is not %q derived from the key", k.ID, freeKeyID)
	}
	return nil
}

// TestFreeTierPinned: the compiled-in free key is the PLUG-69 key file, and checkFreePin rejects a missing, a second and a both-tier free key.
func TestFreeTierPinned(t *testing.T) {
	want, fileSHA := readFreeKeyFile(t)
	if fileSHA != freeKeyFileSH {
		t.Fatalf("testdata key file sha256 = %s, want %s (PLUG-69 evidence)", fileSHA, freeKeyFileSH)
	}
	if !signature.Pinned(signature.TierFree) {
		t.Fatal("the free tier has no pinned key")
	}
	if err := checkFreePin(signature.Keys, signature.Revoked, want); err != nil {
		t.Fatalf("committed table: %v", err)
	}
	if err := signature.CheckKeys(); err != nil {
		t.Fatal(err)
	}
	pinned := signature.Keys[0]
	for _, k := range signature.Keys {
		if k.Tier == signature.TierFree {
			pinned = k
		}
	}
	other, _ := signaturetest.NewKey(t, signature.TierFree)
	both := pinned
	both.Tier = signature.TierLicensed
	flipped := slices.Clone(pinned.PublicKey)
	flipped[0] ^= 1
	for name, table := range map[string][]signature.Key{
		"no free key":            {},
		"second free key":        {pinned, other},
		"key serving both tiers": {pinned, both},
		"free key differs":       {{ID: pinned.ID, Tier: signature.TierFree, PublicKey: flipped}},
		"wrong id":               {{ID: "plugins-0000000000000000", Tier: signature.TierFree, PublicKey: pinned.PublicKey}},
	} {
		if checkFreePin(table, nil, want) == nil {
			t.Errorf("%s: checkFreePin accepted it", name)
		}
	}
	if checkFreePin([]signature.Key{pinned}, []string{pinned.ID}, want) == nil {
		t.Error("a revoked free key was accepted")
	}
}
