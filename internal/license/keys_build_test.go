package license

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/nself-org/cli/internal/compat"
)

// TestKeysCommittedNonZero: the committed ping key set is never empty, each key
// decodes to 32 bytes that are not all zero, and kids are unique and non-empty.
func TestKeysCommittedNonZero(t *testing.T) {
	if len(PingKeys) == 0 {
		t.Fatal("PingKeys is empty")
	}
	seen := map[string]bool{}
	for _, k := range PingKeys {
		if k.ID == "" || seen[k.ID] {
			t.Errorf("kid %q is empty or duplicated", k.ID)
		}
		seen[k.ID] = true
		if len(k.Public) != ed25519.PublicKeySize {
			t.Errorf("kid %s: key is %d bytes, want %d", k.ID, len(k.Public), ed25519.PublicKeySize)
		}
		if bytes.Equal(k.Public, make([]byte, ed25519.PublicKeySize)) {
			t.Errorf("kid %s: key is all zero", k.ID)
		}
	}
}

func TestPubKeyFromHex(t *testing.T) {
	for _, bad := range []string{"", "zz", "abcd", "00"} {
		if pubKeyFromHex(bad) != nil {
			t.Errorf("pubKeyFromHex(%q) should be nil", bad)
		}
	}
	if pubKeyFromHex("0ac4c2d7ec30d23bf2be55775e7170bd24a7544bad036d0d19f2a4deb516e00c") == nil {
		t.Error("valid key rejected")
	}
}

// compatMode15 reports whether the running subtest selected v1.5 (compattest).
func compatMode15() bool { return compat.V15() }

func testKeypairHex(t *testing.T) (string, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(pub), priv
}

func signHex(priv ed25519.PrivateKey, msg []byte) string {
	return hex.EncodeToString(ed25519.Sign(priv, msg))
}
