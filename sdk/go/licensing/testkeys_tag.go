//go:build nself_testkeys

package licensing

import (
	"crypto/ed25519"
	"encoding/hex"
	"os"
)

// Purpose: enable boot-kit test keys. Inputs: test-tag environment only.
// Outputs: tagged key hook. Constraints: excluded from releases. SPORT: plugin.entitlement-token.
func init() {
	taggedTestKey = func() ed25519.PublicKey {
		b, err := hex.DecodeString(os.Getenv("NSELF_ENTITLEMENT_TEST_PUBKEY"))
		if err != nil || len(b) != ed25519.PublicKeySize {
			return nil
		}
		return ed25519.PublicKey(b)
	}
}
