package config

// Purpose: Regression tests for false "unknown env var" warnings on
//          app-owned env vars (ntask dogfood gap #19) and the ENV_ALLOWLIST
//          escape hatch for project-specific vars.

import "testing"

// TestAppOwnedVarsAreKnown asserts the app-owned vars reported by the ntask
// fresh-user sim no longer trigger unknown-var warnings.
func TestAppOwnedVarsAreKnown(t *testing.T) {
	knownSet := make(map[string]bool, len(knownEnvVars))
	for _, k := range knownEnvVars {
		knownSet[k] = true
	}
	for _, v := range []string{
		"NODE_ENV", "JWT_SECRET", "SSL_AUTO_TRUST", "COOKIE_SECRET",
		"ENABLE_DEBUG", "LOG_LEVEL", "NSELF_PROJECT_NAME", "ENV_ALLOWLIST",
		"TOKENS_ENCRYPTION_KEY",
	} {
		if !knownSet[v] {
			t.Errorf("app-owned var %s missing from knownEnvVars (would warn)", v)
		}
	}
}

func TestParseEnvAllowlist(t *testing.T) {
	names, prefixes := parseEnvAllowlist("MY_APP_TOKEN, FEATURE_* ,, OTHER")
	if !names["MY_APP_TOKEN"] || !names["OTHER"] {
		t.Errorf("exact names not parsed: %v", names)
	}
	if len(prefixes) != 1 || prefixes[0] != "FEATURE_" {
		t.Errorf("prefixes = %v, want [FEATURE_]", prefixes)
	}
}

func TestEnvVarAllowlisted(t *testing.T) {
	names, prefixes := parseEnvAllowlist("MY_APP_TOKEN,FEATURE_*")
	cases := map[string]bool{
		"MY_APP_TOKEN":  true,
		"FEATURE_FLAGS": true,
		"FEATURE_":      true,
		"MY_APP_OTHER":  false,
		"RANDOM_VAR":    false,
	}
	for key, want := range cases {
		if got := envVarAllowlisted(key, names, prefixes); got != want {
			t.Errorf("envVarAllowlisted(%q) = %v, want %v", key, got, want)
		}
	}
}
