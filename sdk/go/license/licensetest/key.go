// Package licensetest supplies synthetic signing roots to test-owned fixtures.
package licensetest

import (
	"crypto/ed25519"
	"github.com/nself-org/cli/sdk/go/v2/license/internal/keyset"
	"testing"
)

// Purpose: replace roots for a test. Inputs: test owner and synthetic public key.
// Outputs: automatic cleanup restoration. Constraints: do not use in parallel tests.
// SPORT: plugin.entitlement-token.
func UseKey(t testing.TB, pub ed25519.PublicKey) { t.Helper(); keyset.UseTestingKey(t, pub) }
