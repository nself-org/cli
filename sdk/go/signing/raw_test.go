package signing_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/signing"
)

func TestVerifyEd25519(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("legacy wire payload")
	sig := ed25519.Sign(priv, msg)
	only(t, signing.VerifyEd25519(pub, msg, sig), nil)
	only(t, signing.VerifyEd25519(pub, []byte("tampered"), sig), signing.ErrBadSignature)
	for _, key := range []ed25519.PublicKey{nil, pub[:31], append(append([]byte{}, pub...), 0)} {
		only(t, signing.VerifyEd25519(key, msg, sig), signing.ErrMalformed)
	}
	for _, bad := range [][]byte{nil, sig[:63], append(append([]byte{}, sig...), 0)} {
		only(t, signing.VerifyEd25519(pub, msg, bad), signing.ErrMalformed)
	}
	for _, key := range smallOrderEncodings(t) {
		only(t, signing.VerifyEd25519(key, msg, sig), signing.ErrMalformed)
	}
}
