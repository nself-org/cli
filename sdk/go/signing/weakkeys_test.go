package signing_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/signing"
	"github.com/nself-org/cli/sdk/go/v2/signing/signingtest"
)

// smallOrderEncodings: the 7 libsodium blocklist encodings (sign bit clear).
func smallOrderEncodings(t *testing.T) [][]byte {
	t.Helper()
	hexes := []string{
		strings.Repeat("00", 32),
		"01" + strings.Repeat("00", 31),
		"26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc05",
		"c7176a703d4dd84fba3c0b760d10670f2a2053fa2c39ccc64ec7fd7792ac037a",
		"ec" + strings.Repeat("ff", 30) + "7f",
		"ed" + strings.Repeat("ff", 30) + "7f",
		"ee" + strings.Repeat("ff", 30) + "7f",
	}
	var out [][]byte
	for _, h := range hexes {
		b, err := hex.DecodeString(h)
		if err != nil || len(b) != 32 {
			t.Fatal("bad table entry", h)
		}
		out = append(out, b, append(append([]byte{}, b[:31]...), b[31]|0x80))
	}
	return out
}

// Every small-order encoding (and its sign-bit variant) is refused as
// ErrMalformed at all three entry points, even with a correctly derived id.
func TestSmallOrderKeysRefusedEverywhere(t *testing.T) {
	for i, pub := range smallOrderEncodings(t) {
		id := signing.KeyID(signing.PurposePlugins, pub)
		key := signing.Key{ID: id, Purpose: signing.PurposePlugins, Public: pub}
		_, err := signing.NewVerifier(signing.PurposePlugins, []signing.Key{key}, nil)
		only(t, err, signing.ErrMalformed)

		keys, err := signing.ParseKeysFile(strings.NewReader(id+" "+base64.StdEncoding.EncodeToString(pub)+"\n"), signing.PurposePlugins)
		only(t, err, signing.ErrMalformed)
		if keys != nil {
			t.Fatal("keys returned")
		}

		lv, err := signing.NewLookupVerifier(signing.PurposePlugins, func(context.Context, string) (signing.Key, bool, error) { return key, true, nil })
		if err != nil {
			t.Fatal(err)
		}
		sig := make([]byte, 64)
		copy(sig, pub)
		only(t, lv.Verify([]byte("m"), signing.Signature{KeyID: id, Sig: sig}), signing.ErrMalformed)
		t.Logf("encoding %d refused", i)
	}
}

// Opus review M1: R = identity, S = 0 is a universal signature under the
// identity key. It must not verify anywhere, through any path.
func TestIdentityKeyUniversalForgeryRefused(t *testing.T) {
	ident := make([]byte, 32)
	ident[0] = 1
	sig := make([]byte, 64)
	copy(sig, ident)
	if ed25519.Verify(ident, []byte("anything"), sig) {
		t.Log("stdlib ed25519.Verify accepts the forgery, so the guard is what stops it")
	}
	id := signing.KeyID(signing.PurposePlugins, ident)
	key := signing.Key{ID: id, Purpose: signing.PurposePlugins, Public: ident}
	if v, err := signing.NewVerifier(signing.PurposePlugins, []signing.Key{key}, nil); err == nil {
		for _, m := range []string{"anything", "evil plugin bytes", ""} {
			if v.Verify([]byte(m), signing.Signature{KeyID: id, Sig: sig}) == nil {
				t.Fatalf("FORGERY over %q", m)
			}
		}
	}
	if _, err := signing.ParseKeysFile(strings.NewReader(id+" "+base64.StdEncoding.EncodeToString(ident)), signing.PurposePlugins); err == nil {
		t.Fatal("trust file accepted the identity key")
	}
	lv, _ := signing.NewLookupVerifier(signing.PurposePlugins, func(context.Context, string) (signing.Key, bool, error) { return key, true, nil })
	env := signing.Envelope{PayloadType: "x", Payload: base64.StdEncoding.EncodeToString([]byte("evil")),
		Signatures: []signing.EnvelopeSig{{KeyID: id, Sig: signing.EncodeSig(sig)}}}
	if typ, p, ids, err := signing.VerifyEnvelope(lv, env); err == nil {
		t.Fatalf("FORGERY envelope accepted: %q %q %v", typ, p, ids)
	}
}

// Honest keys are never flagged as small order.
func TestHonestKeysAreNotSmallOrder(t *testing.T) {
	for i := 0; i < 500; i++ {
		pub, _, _ := ed25519.GenerateKey(rand.Reader)
		key := signing.Key{ID: signing.KeyID(signing.PurposeAgent, pub), Purpose: signing.PurposeAgent, Public: pub}
		if _, err := signing.NewVerifier(signing.PurposeAgent, []signing.Key{key}, nil); err != nil {
			t.Fatal(err)
		}
	}
	// Near misses of blocklist entries (one bit off) are ordinary encodings.
	for _, pub := range smallOrderEncodings(t) {
		near := append([]byte{}, pub...)
		near[15] ^= 1
		key := signing.Key{ID: signing.KeyID(signing.PurposeAgent, near), Purpose: signing.PurposeAgent, Public: near}
		if _, err := signing.NewVerifier(signing.PurposeAgent, []signing.Key{key}, nil); err != nil {
			t.Fatalf("near-miss refused: %v", err)
		}
	}
}

// Ids are always derived: a chosen id, another purpose's derivation, or an id
// from different key bytes is refused by NewVerifier and by a lookup.
func TestDerivedIDAlwaysEnforced(t *testing.T) {
	k, _ := signingtest.NewKey(t, signing.PurposePlugins)
	other, _ := signingtest.NewKey(t, signing.PurposePlugins)
	bad := map[string]signing.Key{
		"chosen id":            {ID: "release-2026", Purpose: k.Purpose, Public: k.Public},
		"agent derivation":     {ID: signing.KeyID(signing.PurposeAgent, k.Public), Purpose: k.Purpose, Public: k.Public},
		"agent key as plugins": {ID: signing.KeyID(signing.PurposeAgent, k.Public), Purpose: signing.PurposeAgent, Public: k.Public},
		"other key's id":       {ID: other.ID, Purpose: k.Purpose, Public: k.Public},
		"upper-case id":        {ID: strings.ToUpper(k.ID), Purpose: k.Purpose, Public: k.Public},
	}
	for name, key := range bad {
		t.Run(name, func(t *testing.T) {
			if name == "agent key as plugins" {
				// A well-formed agent key in a plugins verifier is a purpose
				// mismatch at Verify, not a construction error.
				v := newVerifier(t, signing.PurposePlugins, []signing.Key{key}, nil)
				only(t, v.Verify(msg, signing.Signature{KeyID: key.ID, Sig: make([]byte, 64)}), signing.ErrWrongPurpose)
				return
			}
			_, err := signing.NewVerifier(signing.PurposePlugins, []signing.Key{key}, nil)
			only(t, err, signing.ErrMalformed)
			lv, _ := signing.NewLookupVerifier(signing.PurposePlugins, func(context.Context, string) (signing.Key, bool, error) { return key, true, nil })
			only(t, lv.Verify(msg, signing.Signature{KeyID: key.ID, Sig: make([]byte, 64)}), signing.ErrMalformed)
		})
	}
}

// Opus review S1: one public key under two ids must not be listed twice, so
// revoking one id cannot leave a live alias.
func TestDuplicatePublicKeysRefusedAndRevocationHoldsAlias(t *testing.T) {
	k, s := signingtest.NewKey(t, signing.PurposePlugins)
	alias := signing.Key{ID: signing.KeyID(signing.PurposeAgent, k.Public), Purpose: signing.PurposeAgent, Public: k.Public}
	_, err := signing.NewVerifier(signing.PurposePlugins, []signing.Key{k, alias}, nil)
	only(t, err, signing.ErrMalformed)
	// A custom alias id is refused outright.
	custom := signing.Key{ID: "release-2026", Purpose: k.Purpose, Public: k.Public}
	_, err = signing.NewVerifier(signing.PurposePlugins, []signing.Key{k, custom}, nil)
	only(t, err, signing.ErrMalformed)
	// The signature names the alias id; revoking the derived id stops the key
	// everywhere because only the derived id can name it.
	sg := sign(t, s, msg)
	v := newVerifier(t, signing.PurposePlugins, []signing.Key{k}, []string{k.ID})
	only(t, v.Verify(msg, sg), signing.ErrRevoked)
	only(t, v.Verify(msg, signing.Signature{KeyID: "release-2026", Sig: sg.Sig}), signing.ErrUnknownKey)
	// Same in a trust file.
	b := base64.StdEncoding.EncodeToString(k.Public)
	_, err = signing.ParseKeysFile(strings.NewReader(k.ID+" "+b+"\nrelease-2026 "+b+"\n"), signing.PurposePlugins)
	only(t, err, signing.ErrMalformed)
}

func TestNilVerifierMethods(t *testing.T) {
	var v *signing.Verifier
	if v.Purpose() != "" {
		t.Fatal("nil Purpose")
	}
	only(t, v.Verify(msg, signing.Signature{}), signing.ErrMalformed)
	only(t, v.VerifyContext(context.Background(), msg, signing.Signature{}), signing.ErrMalformed)
	_, _, _, err := signing.VerifyEnvelope(v, signing.Envelope{})
	only(t, err, signing.ErrMalformed)
	var z signing.Verifier
	if z.Purpose() != "" {
		t.Fatal("zero Purpose")
	}
}
