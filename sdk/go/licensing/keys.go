// Package licensing verifies the ping-issued runtime entitlement contract.
package licensing

import (
	"crypto/ed25519"
	"github.com/nself-org/cli/sdk/go/v2/licensing/internal/keyset"
)

// PingKey identifies a ping Ed25519 verification key.
type PingKey struct {
	ID     string
	Public ed25519.PublicKey
}

// Purpose: expose verification roots for parity checks. Inputs: none.
// Outputs: defensive copies. Constraints: no setter or env override.
// SPORT: plugin.entitlement-token.
func PingKeys() []PingKey {
	snapshot := keyset.Snapshot()
	result := make([]PingKey, len(snapshot))
	for i, k := range snapshot {
		result[i] = PingKey{k.ID, k.Public}
	}
	return result
}

var taggedTestKey func() ed25519.PublicKey

// Purpose: select a signature root. Inputs: signed kid. Outputs: public key.
// Constraints: only the test build tag permits a test root. SPORT: plugin.entitlement-token.
func signingKey(kid string) ed25519.PublicKey {
	if kid == "1" && taggedTestKey != nil {
		if key := taggedTestKey(); usableKey(key) {
			return key
		}
	}
	for _, key := range keyset.Snapshot() {
		if kid != "" && key.ID == kid && usableKey(key.Public) {
			return key.Public
		}
	}
	return nil
}

// Purpose: reject empty roots. Inputs: public key. Outputs: nonzero sized root.
// Constraints: no zero-key verification bypass. SPORT: plugin.entitlement-token.
func usableKey(key ed25519.PublicKey) bool {
	if len(key) != ed25519.PublicKeySize {
		return false
	}
	for _, b := range key {
		if b != 0 {
			return true
		}
	}
	return false
}
