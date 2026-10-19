package secrets

import (
	"strings"
	"testing"
)

// D-0660: AUTH_ENCRYPTION_KEY is never given a generated replacement.
func TestRotationDeclinesAuthEncryptionKey(t *testing.T) {
	for _, tc := range []struct {
		key       string
		wantEmpty bool
	}{
		{"AUTH_ENCRYPTION_KEY", true},
		{"auth_encryption_key", true},
		{"AUTH_ENCRYPTION_KEY_PREVIOUS", false}, // not the key itself
		{"POSTGRES_PASSWORD", false},
		{"HASURA_GRAPHQL_ADMIN_SECRET", false},
	} {
		value, hint := generateRotationValue(tc.key)
		if (value == "") != tc.wantEmpty {
			t.Errorf("%s: value empty = %v, want %v", tc.key, value == "", tc.wantEmpty)
		}
		if tc.wantEmpty && !strings.Contains(hint, "TOTP") {
			t.Errorf("%s: hint %q should say why", tc.key, hint)
		}
	}
}
