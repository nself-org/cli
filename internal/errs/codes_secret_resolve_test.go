package errs

// Purpose: verify the secret resolution error registry fragment.
// Inputs: registered E540-E544 entries.
// Outputs: assertions on metadata and owner range.
// Constraints: no values in error descriptions.
// SPORT: P7-TRUST-03.

import (
	"fmt"
	"testing"
)

// TestSecretResolveCodes proves E540-E544 have registered descriptions, fixes and doc anchors.
func TestSecretResolveCodes(t *testing.T) {
	for i := 540; i <= 544; i++ {
		code := fmt.Sprintf("E%d", i)
		e, ok := Registry[code]
		if !ok || e.Category != "secret" || e.Summary == "" || e.DefaultWhy == "" || e.DefaultFix == "" || e.DocsPath != fmt.Sprintf("reference/error-codes#e%d", i) || e.Exit != 1 {
			t.Errorf("invalid entry %s: %+v", code, e)
		}
	}
	owner := Registry["E544"]
	if owner.Summary != "Secret class or recipient change not permitted" ||
		owner.DefaultWhy != "Only the store owner may change recipients, the owner cannot be removed, and release or deploy changes require explicit release-class consent." ||
		owner.DefaultFix != "Run recipient changes with the store owner identity, preserve its recipient, and use --allow-release-class only for an eligible dedicated trusted node. Use secrets rekey for owner changes." {
		t.Errorf("E544 must preserve owner-only refusal and release consent guidance: %+v", owner)
	}
	if e := RegistryErrors(); len(e) != 0 {
		t.Fatal(e)
	}
}
