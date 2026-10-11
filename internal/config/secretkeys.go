package config

// Purpose: one rule for "this config key names a secret", shared by the CLI
// masking helpers and the machine audit log (P7-SURF-25).
// Inputs: a config key or flag name.
// Outputs: true when the name contains SECRET, PASSWORD, KEY or TOKEN
// (case-insensitive).
// Constraints: the rule is the cmd/commands isSecretKey basis rule, moved
// here unchanged; P7-SURF-12 deletes the cmd copy. Over-matching (for example
// PUBLIC_KEY) is the safe side: a match only redacts.

import "strings"

// secretKeyParts are the substrings whose presence in an upper-cased key name
// marks a secret value.
var secretKeyParts = []string{"SECRET", "PASSWORD", "KEY", "TOKEN"}

// IsSecretKey reports whether key names a secret value.
func IsSecretKey(key string) bool {
	upper := strings.ToUpper(key)
	for _, part := range secretKeyParts {
		if strings.Contains(upper, part) {
			return true
		}
	}
	return false
}
