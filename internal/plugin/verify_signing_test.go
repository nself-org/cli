package plugin

import (
	"encoding/hex"
	"testing"
)

func TestCanonicalVerificationRejectsSmallOrderKey(t *testing.T) {
	// The identity point with R = identity, S = 0 verifies any message in
	// crypto/ed25519. All verification paths must reject this public key.
	pub, sig := make([]byte, 32), make([]byte, 64)
	pub[0], sig[0] = 1, 1
	err := VerifyCanonicalSignature(goldenBundle, goldenName, goldenSHA256, goldenVis,
		goldenVersion, goldenSize, hex.EncodeToString(pub), hex.EncodeToString(sig))
	if err == nil {
		t.Fatal("accepted a universal forgery under a small-order key")
	}
}
