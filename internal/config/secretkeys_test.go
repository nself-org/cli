package config

import "testing"

// TestIsSecretKey pins IsSecretKey to the answers of the basis rule
// (cmd/commands isSecretKey at cli f6c45e2f: upper-case the key, then match
// SECRET, PASSWORD, KEY or TOKEN as a substring) for 20 keys.
func TestIsSecretKey(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{"POSTGRES_PASSWORD", true},
		{"HASURA_GRAPHQL_ADMIN_SECRET", true},
		{"HASURA_GRAPHQL_JWT_SECRET", true},
		{"STRIPE_SECRET_KEY", true},
		{"NSELF_PLUGIN_LICENSE_KEY", true},
		{"NSELF_MCP_TOKEN", true},
		{"MINIO_ROOT_PASSWORD", true},
		{"S3_ACCESS_KEY", true},
		{"postgres_password", true},
		{"value", false},
		{"api-key", true},
		{"monkey", true},
		{"tokens", true},
		{"PROJECT_NAME", false},
		{"ENV", false},
		{"BASE_DOMAIN", false},
		{"POSTGRES_USER", false},
		{"HASURA_GRAPHQL_ENABLE_CONSOLE", false},
		{"PASSWD", false},
		{"", false},
	}
	if len(cases) != 20 {
		t.Fatalf("table has %d keys, want 20", len(cases))
	}
	for _, c := range cases {
		if got := IsSecretKey(c.key); got != c.want {
			t.Errorf("IsSecretKey(%q) = %v, want %v", c.key, got, c.want)
		}
	}
}
