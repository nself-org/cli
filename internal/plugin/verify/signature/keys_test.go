package signature_test

// keys_test.go — the pinned production release keys of both tiers (P7-PLUG-71 free, P7-PLUG-72 licensed).
//
// Purpose: prove each compiled-in key is exactly the public key file its first signed release committed, and that the table cannot silently lose a tier, double a tier's key, or let one key serve both tiers.
// Inputs: testdata/real/plugins-release-free-2ebca8dfad98e19e.pub (byte copy of plugins:scripts/release/keys/ at the P7-PLUG-69 release) and testdata/real/plugins-release-licensed-plugins-e2ac3ee72a143603.pub (byte copy of bundles:scripts/release/keys/ at the P7-PLUG-70 release).
// Outputs: test results only.
// Constraints: public keys only.

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

// pin is one tier's expected production key: the committed key file, its sha256 (from the release evidence) and the id derived from it.
type pin struct {
	tier, file, id, fileSHA string
}

var pins = []pin{
	{signature.TierFree, "testdata/real/plugins-release-free-2ebca8dfad98e19e.pub", "plugins-2ebca8dfad98e19e", "6720a63381e247173ed79f9cdf49e82df0e3f2ed4e7fa7a897b5bde6c6e84a4a"},                 // PLUG-69 evidence key-id, key-sha256
	{signature.TierLicensed, "testdata/real/plugins-release-licensed-plugins-e2ac3ee72a143603.pub", "plugins-e2ac3ee72a143603", "6b532fac5c7b681d53071fc585fd9fa69cf6dbe271eae40fdde43bace598ab4d"}, // PLUG-70 evidence key-id, key-sha256
}

// readKeyFile returns the raw Ed25519 key and the sha256 of the committed key file.
func readKeyFile(t *testing.T, file string) (ed25519.PublicKey, string) {
	t.Helper()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "PUBLIC KEY" {
		t.Fatalf("%s is not a PEM PUBLIC KEY", file)
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	ed, ok := pub.(ed25519.PublicKey)
	if !ok {
		t.Fatalf("%s holds a %T, want an Ed25519 key", file, pub)
	}
	return ed, hex.EncodeToString(sum[:])
}

// checkPins reports why table is not a correct pin of both tiers: per tier exactly one key, equal to want[tier], its id derived from it and equal to the expected id,
// no public key listed under two tiers, none of the keys revoked.
func checkPins(table []signature.Key, revoked []string, want map[string]ed25519.PublicKey) error {
	byTier := map[string][]signature.Key{}
	tiers := map[string]map[string]bool{} // raw key -> tiers it serves
	for _, k := range table {
		if tiers[string(k.PublicKey)] == nil {
			tiers[string(k.PublicKey)] = map[string]bool{}
		}
		tiers[string(k.PublicKey)][k.Tier] = true
		byTier[k.Tier] = append(byTier[k.Tier], k)
		if slices.Contains(revoked, k.ID) {
			return fmt.Errorf("key %s is revoked", k.ID)
		}
	}
	for _, ts := range tiers {
		if len(ts) > 1 {
			return errors.New("one public key serves both tiers")
		}
	}
	for _, p := range pins {
		ks := byTier[p.tier]
		if len(ks) != 1 {
			return fmt.Errorf("want exactly 1 %s key, have %d", p.tier, len(ks))
		}
		k := ks[0]
		if !bytes.Equal(k.PublicKey, want[p.tier]) {
			return fmt.Errorf("%s key differs from the committed key file", p.tier)
		}
		if k.ID != signing.KeyID(signing.PurposePlugins, k.PublicKey) || k.ID != p.id {
			return fmt.Errorf("%s key id %q is not %q derived from the key", p.tier, k.ID, p.id)
		}
	}
	if len(table) != len(pins) {
		return fmt.Errorf("table holds %d keys, want %d", len(table), len(pins))
	}
	return nil
}

// keyOf returns the table's key for tier.
func keyOf(t *testing.T, tier string) signature.Key {
	t.Helper()
	for _, k := range signature.Keys {
		if k.Tier == tier {
			return k
		}
	}
	t.Fatalf("no %s key in the compiled-in table", tier)
	return signature.Key{}
}

// TestPinnedTiers: the compiled-in table holds exactly the free and the licensed production key, each equal to its committed key file, and checkPins rejects a
// missing tier, a second key for a tier, a key serving both tiers, a changed key, a wrong id and a revoked key.
func TestPinnedTiers(t *testing.T) {
	want := map[string]ed25519.PublicKey{}
	for _, p := range pins {
		pub, fileSHA := readKeyFile(t, p.file)
		if fileSHA != p.fileSHA {
			t.Fatalf("%s key file sha256 = %s, want %s (release evidence)", p.tier, fileSHA, p.fileSHA)
		}
		if !signature.Pinned(p.tier) {
			t.Fatalf("the %s tier has no pinned key", p.tier)
		}
		want[p.tier] = pub
	}
	if err := checkPins(signature.Keys, signature.Revoked, want); err != nil {
		t.Fatalf("committed table: %v", err)
	}
	if err := signature.CheckKeys(); err != nil {
		t.Fatal(err)
	}
	free, licensed := keyOf(t, signature.TierFree), keyOf(t, signature.TierLicensed)
	otherFree, _ := signaturetest.NewKey(t, signature.TierFree)
	otherLicensed, _ := signaturetest.NewKey(t, signature.TierLicensed)
	freeAsLicensed, licensedAsFree := free, licensed
	freeAsLicensed.Tier, licensedAsFree.Tier = signature.TierLicensed, signature.TierFree
	flip := func(k signature.Key) signature.Key {
		k.PublicKey = slices.Clone(k.PublicKey)
		k.PublicKey[0] ^= 1
		return k
	}
	wrongID := func(k signature.Key) signature.Key { k.ID = "plugins-0000000000000000"; return k }
	for name, table := range map[string][]signature.Key{
		"no free key":                {licensed},
		"no licensed key":            {free},
		"empty table":                {},
		"second free key":            {free, licensed, otherFree},
		"second licensed key":        {free, licensed, otherLicensed},
		"free key serves both tiers": {free, licensed, freeAsLicensed},
		"licensed key serves both":   {free, licensed, licensedAsFree},
		"free key under licensed":    {freeAsLicensed, licensed},
		"free key differs":           {flip(free), licensed},
		"licensed key differs":       {free, flip(licensed)},
		"free wrong id":              {wrongID(free), licensed},
		"licensed wrong id":          {free, wrongID(licensed)},
	} {
		if checkPins(table, nil, want) == nil {
			t.Errorf("%s: checkPins accepted it", name)
		}
	}
	for _, k := range []signature.Key{free, licensed} {
		if checkPins(signature.Keys, []string{k.ID}, want) == nil {
			t.Errorf("a revoked %s key was accepted", k.Tier)
		}
	}
}
