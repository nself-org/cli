package signing_test

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/signing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The committed fixture was signed by `openssl pkeyutl -sign -rawin` over a
// domain-separated string; it verifies, and no other message does.
func TestOpenSSLFixtureVerifies(t *testing.T) {
	pubRaw, err := base64.StdEncoding.Strict().DecodeString(string(fixture(t, "fixture.pub.b64")))
	if err != nil {
		t.Fatal(err)
	}
	pub := ed25519.PublicKey(pubRaw)
	key := signing.Key{ID: signing.KeyID(signing.PurposePlugins, pub), Purpose: signing.PurposePlugins, Public: pub}
	sig, err := signing.DecodeSig(string(fixture(t, "fixture.sig.b64")))
	if err != nil {
		t.Fatal(err)
	}
	m := fixture(t, "fixture.msg")
	v := newVerifier(t, signing.PurposePlugins, []signing.Key{key}, nil)
	only(t, v.Verify(m, signing.Signature{KeyID: key.ID, Sig: sig}), nil)
	only(t, v.Verify(append([]byte("x"), m...), signing.Signature{KeyID: key.ID, Sig: sig}), signing.ErrBadSignature)
	only(t, newVerifier(t, signing.PurposeAgent, []signing.Key{key}, nil).Verify(m, signing.Signature{KeyID: key.ID, Sig: sig}), signing.ErrWrongPurpose)
}

// A PKCS#8 PEM from `openssl genpkey -algorithm ed25519` parses, belongs to the
// fixture public key, and signs a message the verifier accepts.
func TestParsePKCS8PEMFixture(t *testing.T) {
	pemBytes := fixture(t, "fixture-ed25519.pkcs8.pem")
	priv, err := signing.ParsePKCS8PEM(pemBytes)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := base64.StdEncoding.DecodeString(string(fixture(t, "fixture.pub.b64")))
	pub := priv.Public().(ed25519.PublicKey)
	if string(pub) != string(want) {
		t.Fatal("parsed key is not the fixture key")
	}
	id := signing.KeyID(signing.PurposeAgent, pub)
	s := signing.NewEd25519Signer(id, priv)
	if s.KeyID() != id {
		t.Fatal("KeyID")
	}
	m := []byte("signed by a parsed key")
	sg := sign(t, s, m)
	v := newVerifier(t, signing.PurposeAgent, []signing.Key{{ID: id, Purpose: signing.PurposeAgent, Public: pub}}, nil)
	only(t, v.Verify(m, sg), nil)
	// Leading and trailing whitespace around the block is fine.
	if _, err := signing.ParsePKCS8PEM(append(append([]byte("\n\n"), pemBytes...), "\n \n"...)); err != nil {
		t.Fatal(err)
	}
	// Ed25519 signatures are deterministic: same bytes as the fixture's openssl run.
	fm := fixture(t, "fixture.msg")
	got, err := signing.NewEd25519Signer("k", priv).Sign(fm)
	if err != nil {
		t.Fatal(err)
	}
	if signing.EncodeSig(got) != string(fixture(t, "fixture.sig.b64")) {
		t.Fatal("Go signature differs from the openssl signature")
	}
}

func TestParsePKCS8PEMRejects(t *testing.T) {
	good := fixture(t, "fixture-ed25519.pkcs8.pem")
	ec, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ecDER, _ := x509.MarshalPKCS8PrivateKey(ec)
	ecPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ecDER})
	blk, _ := pem.Decode(good)
	hdr := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Headers: map[string]string{"X": "y"}, Bytes: blk.Bytes})
	typ := pem.EncodeToMemory(&pem.Block{Type: "ED25519 PRIVATE KEY", Bytes: blk.Bytes})
	trunc := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: blk.Bytes[:len(blk.Bytes)-1]})
	cases := map[string][]byte{
		"nil":          nil,
		"empty":        {},
		"garbage":      []byte("not pem"),
		"two blocks":   append(append([]byte{}, good...), good...),
		"trailing":     append(append([]byte{}, good...), "junk"...),
		"ecdsa":        ecPEM,
		"headers":      hdr,
		"wrong type":   typ,
		"truncated":    trunc,
		"public block": pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: blk.Bytes}),
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			priv, err := signing.ParsePKCS8PEM(in)
			only(t, err, signing.ErrMalformed)
			if priv != nil {
				t.Fatal("key returned on error")
			}
			if strings.Contains(err.Error(), base64.StdEncoding.EncodeToString(blk.Bytes)) {
				t.Fatal("error leaks key bytes")
			}
		})
	}
}

func TestSignerNeverPanics(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	for name, s := range map[string]signing.Signer{
		"nil key":   signing.NewEd25519Signer("ok-id", nil),
		"short key": signing.NewEd25519Signer("ok-id", priv[:10]),
		"long key":  signing.NewEd25519Signer("ok-id", append(append([]byte{}, priv...), 1)),
		"empty id":  signing.NewEd25519Signer("", priv),
		"bad id":    signing.NewEd25519Signer("a b", priv),
		"long id":   signing.NewEd25519Signer(strings.Repeat("a", 129), priv),
	} {
		t.Run(name, func(t *testing.T) {
			sig, err := s.Sign([]byte("m"))
			only(t, err, signing.ErrMalformed)
			if sig != nil {
				t.Fatal("signature returned on error")
			}
		})
	}
	if sig, err := signing.NewEd25519Signer(strings.Repeat("a", 128), priv).Sign(nil); err != nil || len(sig) != 64 {
		t.Fatalf("128-char id and empty message must sign: %v", err)
	}
}

func TestEncodeDecodeSig(t *testing.T) {
	raw := make([]byte, 64)
	for i := range raw {
		raw[i] = byte(i * 7)
	}
	enc := signing.EncodeSig(raw)
	if enc != base64.StdEncoding.EncodeToString(raw) || len(enc) != 88 {
		t.Fatalf("EncodeSig = %q", enc)
	}
	back, err := signing.DecodeSig(enc)
	if err != nil || string(back) != string(raw) {
		t.Fatalf("round trip: %v", err)
	}
	// Strictness: every non-canonical spelling is malformed.
	url := base64.URLEncoding.EncodeToString([]byte(strings.Repeat("\xfb\xff", 32)))
	cases := map[string]string{
		"empty":        "",
		"newline":      enc + "\n",
		"embedded nl":  enc[:40] + "\n" + enc[40:],
		"cr":           enc + "\r",
		"space":        enc + " ",
		"no padding":   strings.TrimRight(enc, "="),
		"url alphabet": url,
		"short":        signing.EncodeSig(raw[:63]),
		"long":         signing.EncodeSig(append(append([]byte{}, raw...), 0)),
		"not base64":   "!!!!",
		"nul":          enc[:10] + "\x00" + enc[11:],
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			b, err := signing.DecodeSig(in)
			only(t, err, signing.ErrMalformed)
			if b != nil {
				t.Fatal("bytes returned on error")
			}
		})
	}
	// Non-zero trailing bits: of the 64 alphabet characters in the last data
	// position exactly the 4 with zero low bits are canonical.
	alphabet := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	ok := 0
	for i := 0; i < len(alphabet); i++ {
		if _, err := signing.DecodeSig(enc[:85] + alphabet[i:i+1] + "=="); err == nil {
			ok++
		}
	}
	if ok != 4 {
		t.Fatalf("%d canonical final characters, want 4", ok)
	}
}
