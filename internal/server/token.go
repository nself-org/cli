package server

// Purpose: resolve a Hetzner Cloud API token without ever hardcoding one.
// Mirrors the project-scoped vault pattern already in use for this repo
// (HETZNER_NSELF_TOKEN, documented in cli/.claude/CLAUDE.md's Vault
// Variables table) and generalizes it so any ~/Sites project's own token
// var (HETZNER_UNYECO_TOKEN, HETZNER_UMMECO_TOKEN, ...) works the same way
// without a code change — only the --token-env value differs.
// Inputs: an explicit --token flag value (highest priority, for one-off use
// or CI secrets), an env var name to check (--token-env, defaults to
// HETZNER_NSELF_TOKEN), and the process environment.
// Outputs: the resolved token, or an error naming exactly what was checked.
// Constraints: never logs or echoes the token value itself.

import (
	"fmt"
	"os"
)

// DefaultTokenEnvVar is the vault-documented env var for this repo's own
// Hetzner project. Other ~/Sites projects pass their own var name via
// --token-env (e.g. HETZNER_UNYECO_TOKEN) rather than needing a code change.
const DefaultTokenEnvVar = "HETZNER_NSELF_TOKEN"

// legacyTokenEnvVar is hcloud's own conventional var name, checked as a
// fallback so a shell already set up for the hcloud CLI works unmodified.
const legacyTokenEnvVar = "HCLOUD_TOKEN"

// ResolveToken returns the Hetzner API token to use, in priority order:
// explicit (the --token flag) > envVar (--token-env, or DefaultTokenEnvVar
// if empty) > legacyTokenEnvVar. It never reads a token from a file or
// hardcodes one — vault.env is expected to be sourced into the process
// environment before nself runs, per this repo's credential doctrine.
func ResolveToken(explicit, envVar string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if envVar == "" {
		envVar = DefaultTokenEnvVar
	}
	if v := os.Getenv(envVar); v != "" {
		return v, nil
	}
	if v := os.Getenv(legacyTokenEnvVar); v != "" {
		return v, nil
	}
	return "", fmt.Errorf(
		"no Hetzner API token found: set %s (or %s) in the environment, or pass --token explicitly",
		envVar, legacyTokenEnvVar)
}
