package server

import (
	"os"
	"strings"
	"testing"
)

func TestResolveToken_ExplicitWins(t *testing.T) {
	t.Setenv(DefaultTokenEnvVar, "from-env")
	got, err := ResolveToken("from-flag", "")
	if err != nil {
		t.Fatalf("ResolveToken: %v", err)
	}
	if got != "from-flag" {
		t.Errorf("got %q, want explicit flag value to win", got)
	}
}

func TestResolveToken_DefaultEnvVar(t *testing.T) {
	os.Unsetenv(legacyTokenEnvVar)
	t.Setenv(DefaultTokenEnvVar, "vault-token")
	got, err := ResolveToken("", "")
	if err != nil {
		t.Fatalf("ResolveToken: %v", err)
	}
	if got != "vault-token" {
		t.Errorf("got %q, want %s value", got, DefaultTokenEnvVar)
	}
}

func TestResolveToken_ProjectScopedEnvVar(t *testing.T) {
	os.Unsetenv(legacyTokenEnvVar)
	os.Unsetenv(DefaultTokenEnvVar)
	t.Setenv("HETZNER_UNYECO_TOKEN", "unyeco-token")
	got, err := ResolveToken("", "HETZNER_UNYECO_TOKEN")
	if err != nil {
		t.Fatalf("ResolveToken: %v", err)
	}
	if got != "unyeco-token" {
		t.Errorf("got %q, want the project-scoped env var honored", got)
	}
}

func TestResolveToken_LegacyFallback(t *testing.T) {
	os.Unsetenv(DefaultTokenEnvVar)
	t.Setenv(legacyTokenEnvVar, "hcloud-cli-token")
	got, err := ResolveToken("", "")
	if err != nil {
		t.Fatalf("ResolveToken: %v", err)
	}
	if got != "hcloud-cli-token" {
		t.Errorf("got %q, want HCLOUD_TOKEN fallback", got)
	}
}

func TestResolveToken_NoneSet_ErrorNamesWhatWasChecked(t *testing.T) {
	os.Unsetenv(DefaultTokenEnvVar)
	os.Unsetenv(legacyTokenEnvVar)
	_, err := ResolveToken("", "")
	if err == nil {
		t.Fatal("ResolveToken: want error when no token is available anywhere")
	}
	if !strings.Contains(err.Error(), DefaultTokenEnvVar) {
		t.Errorf("error %q should name the env var it checked", err.Error())
	}
}
