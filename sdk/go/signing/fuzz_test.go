package signing_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/signing"
)

func isSentinel(err error) bool {
	for _, s := range allSentinels {
		if errors.Is(err, s) {
			return true
		}
	}
	return false
}

// FuzzVerify: arbitrary key id, signature bytes and message never panic and
// only ever produce a sentinel error or a genuine verification.
func FuzzVerify(f *testing.F) {
	k := fuzzKey(f, signing.PurposeCIRelease)
	v, err := signing.NewVerifier(signing.PurposeCIRelease, []signing.Key{k}, []string{"ci-release-0123456789abcdef"})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(k.ID, make([]byte, 64), []byte("m"))
	f.Add("", []byte{}, []byte{})
	f.Add("ci-release-0123456789abcdef", make([]byte, 64), []byte("m"))
	f.Add(strings.Repeat("a", 500), make([]byte, 65), []byte("\x00"))
	f.Fuzz(func(t *testing.T, id string, sig, m []byte) {
		err := v.Verify(m, signing.Signature{KeyID: id, Sig: sig})
		if err == nil {
			if id != k.ID || !ed25519.Verify(k.Public, m, sig) {
				t.Fatal("accepted a signature that does not verify")
			}
			return
		}
		if !isSentinel(err) {
			t.Fatalf("non-sentinel error %v", err)
		}
	})
}

func FuzzDecodeSig(f *testing.F) {
	f.Add(base64.StdEncoding.EncodeToString(make([]byte, 64)))
	f.Add("")
	f.Add("AAAA\n")
	f.Fuzz(func(t *testing.T, in string) {
		b, err := signing.DecodeSig(in)
		if err != nil {
			if !errors.Is(err, signing.ErrMalformed) || b != nil {
				t.Fatalf("bad failure: %v", err)
			}
			return
		}
		if len(b) != 64 || signing.EncodeSig(b) != in {
			t.Fatalf("accepted a non-canonical signature %q", in)
		}
	})
}

func FuzzParseKeysFile(f *testing.F) {
	_, _, e := entry(signing.PurposePlugins, "seed")
	f.Add(e + "\n# c\n")
	f.Add("")
	f.Add("a\r\n\x00")
	f.Fuzz(func(t *testing.T, in string) {
		keys, err := signing.ParseKeysFile(strings.NewReader(in), signing.PurposePlugins)
		if err != nil {
			if !errors.Is(err, signing.ErrMalformed) || keys != nil {
				t.Fatalf("bad failure: %v", err)
			}
			return
		}
		seen := map[string]bool{}
		for _, k := range keys {
			if seen[k.ID] || len(k.Public) != 32 || k.ID == "" {
				t.Fatalf("invalid key accepted: %+v", k)
			}
			seen[k.ID] = true
		}
		if _, err := signing.NewVerifier(signing.PurposePlugins, keys, nil); err != nil {
			t.Fatalf("parsed keys rejected by NewVerifier: %v", err)
		}
	})
}

func FuzzParseRevokedFile(f *testing.F) {
	f.Add("ci-release-0123456789abcdef\nplugins-0123456789abcdef\n")
	f.Add("a b")
	f.Fuzz(func(t *testing.T, in string) {
		ids, err := signing.ParseRevokedFile(strings.NewReader(in))
		if err != nil {
			if !errors.Is(err, signing.ErrMalformed) || ids != nil {
				t.Fatalf("bad failure: %v", err)
			}
			return
		}
		if _, err := signing.NewVerifier(signing.PurposePlugins, nil, ids); err != nil {
			t.Fatalf("parsed ids rejected: %v", err)
		}
	})
}

func FuzzParsePKCS8PEM(f *testing.F) {
	f.Add([]byte("-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----\n"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, in []byte) {
		priv, err := signing.ParsePKCS8PEM(in)
		if err != nil && (priv != nil || !errors.Is(err, signing.ErrMalformed)) {
			t.Fatalf("bad failure: %v", err)
		}
		if err == nil && len(priv) != ed25519.PrivateKeySize {
			t.Fatal("short key accepted")
		}
	})
}

// FuzzVerifyEnvelope: any JSON-shaped envelope never panics and returns no
// payload unless verification succeeded.
func FuzzVerifyEnvelope(f *testing.F) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	k := signing.Key{ID: signing.KeyID(signing.PurposeCIRelease, pub), Purpose: signing.PurposeCIRelease, Public: pub}
	s := signing.NewEd25519Signer(signing.PurposeCIRelease, priv)
	v, err := signing.NewVerifier(signing.PurposeCIRelease, []signing.Key{k}, nil)
	if err != nil {
		f.Fatal(err)
	}
	env, _ := signing.SignEnvelope(s, pt, []byte("p"))
	seed, _ := json.Marshal(env)
	f.Add(seed)
	f.Add([]byte(`{"payloadType":"a","payload":"","signatures":[]}`))
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, in []byte) {
		var e signing.Envelope
		if json.Unmarshal(in, &e) != nil {
			return
		}
		typ, payload, ids, err := signing.VerifyEnvelope(v, e)
		if err != nil {
			if typ != "" || payload != nil || ids != nil || !isSentinel(err) {
				t.Fatalf("bad failure: %q %q %v", typ, payload, err)
			}
			return
		}
		if len(ids) == 0 {
			t.Fatal("success without a verified key id")
		}
		if typ != e.PayloadType {
			t.Fatal("type mismatch")
		}
		raw, derr := base64.StdEncoding.DecodeString(e.Payload)
		if derr != nil || string(raw) != string(payload) {
			t.Fatal("payload mismatch")
		}
	})
}

func fuzzKey(f *testing.F, p signing.Purpose) signing.Key {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		f.Fatal(err)
	}
	return signing.Key{ID: signing.KeyID(p, pub), Purpose: p, Public: pub}
}
