// Package keyset owns the SDK's immutable production verification roots.
package keyset

import (
	"crypto/ed25519"
	"encoding/hex"
	"sync"
	"testing"
)

// Key identifies a committed ping signing public key.
type Key struct {
	ID     string
	Public ed25519.PublicKey
}

var mu sync.RWMutex
var keys = []Key{{ID: "1", Public: decode("0ac4c2d7ec30d23bf2be55775e7170bd24a7544bad036d0d19f2a4deb516e00c")}}

// Purpose: decode public roots. Inputs: hex. Outputs: key or nil.
// Constraints: malformed roots fail closed. SPORT: plugin.entitlement-token.
func decode(value string) ed25519.PublicKey {
	b, err := hex.DecodeString(value)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil
	}
	return ed25519.PublicKey(b)
}

// Purpose: snapshot roots. Inputs: none. Outputs: independent copies.
// Constraints: callers cannot mutate roots. SPORT: plugin.entitlement-token.
func Snapshot() []Key {
	mu.RLock()
	defer mu.RUnlock()
	result := make([]Key, len(keys))
	for i, k := range keys {
		result[i] = Key{k.ID, append(ed25519.PublicKey(nil), k.Public...)}
	}
	return result
}

// Purpose: support synthetic-key tests. Inputs: testing owner and public key.
// Outputs: restored roots at cleanup. Constraints: not parallel-safe across tests.
// SPORT: plugin.entitlement-token.
func UseTestingKey(t testing.TB, pub ed25519.PublicKey) {
	t.Helper()
	if len(pub) != ed25519.PublicKeySize {
		t.Fatal("invalid synthetic public key")
	}
	old := swap([]Key{{"1", append(ed25519.PublicKey(nil), pub...)}})
	t.Cleanup(func() { swap(old) })
}

// Purpose: swap test-owned roots. Inputs: replacement. Outputs: prior roots.
// Constraints: package-private mutation hook. SPORT: plugin.entitlement-token.
func swap(replacement []Key) []Key {
	mu.Lock()
	defer mu.Unlock()
	old := keys
	keys = replacement
	return old
}
