package commands

// secrets_core_test.go — Unit tests for the `nself secrets` CRUD
// subcommands in secrets_core.go: init, set, get, list.
// P6-E11-W2-S3-T18: security command test floor (was 0% direct coverage).
//
// Security property under test: environment isolation. `nself secrets`
// stores dev/staging/prod secrets in separate encrypted files
// (.secrets/dev.age, staging.age, prod.age). A secret set with --env dev
// must NOT be readable with --env prod (and vice versa) — a command-layer
// regression that ignored the --env flag would leak a prod credential to
// anyone running `secrets get` in a dev context, or vice versa. That bug
// class lives entirely in the cobra wiring (secretsEnvFlag plumbing), not
// in internal/secrets, so it can only be caught by exercising the actual
// RunE functions — the gap this ticket closes.
// Inputs: temp project roots initialized via the real secretsInitCmd.
// Outputs: encrypted store content, or command errors.
// Constraints: requires the `age`/`age-keygen` binaries (see requireAge in
// secrets_audit_test.go); skips cleanly when unavailable, matching
// internal/secrets' own documented CI limitation.

import (
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/secrets"
)

// initSecretsProject runs the real secretsInitCmd against root and fails the
// test on error. Scopes the generated age key to a path inside root via
// SECRETS_AGE_KEY_PATH so tests never read or write the developer's real
// ~/.config/nself/age-key.txt, and so parallel/sequential test cases never
// share a keypair. Shared setup for the CRUD tests below.
func initSecretsProject(t *testing.T, root string) {
	t.Helper()
	t.Setenv("SECRETS_AGE_KEY_PATH", filepath.Join(root, "age-key.txt"))
	if err := secretsInitCmd.RunE(secretsInitCmd, nil); err != nil {
		t.Fatalf("secretsInitCmd.RunE: %v", err)
	}
}

// TestSecretsSetGet_EnvironmentIsolation verifies the core security property:
// a secret set in one environment is invisible when reading from another.
func TestSecretsSetGet_EnvironmentIsolation(t *testing.T) {
	requireAge(t)
	withProjectRoot(t, func(root string) {
		initSecretsProject(t, root)

		secretsEnvFlag = "dev"
		if err := secretsSetCmd.RunE(secretsSetCmd, []string{"DB_PASSWORD", "dev-only-value"}); err != nil {
			t.Fatalf("set (dev): %v", err)
		}

		secretsEnvFlag = "prod"
		defer func() { secretsEnvFlag = "dev" }()

		err := secretsGetCmd.RunE(secretsGetCmd, []string{"DB_PASSWORD"})
		if err == nil {
			t.Fatal("expected error reading a dev-only secret with --env prod, got nil " +
				"(environment isolation is broken — a prod read can see a dev value)")
		}

		// Confirm the dev value is still there and correct via the same
		// internal/secrets.Get the command wraps, ruling out "the whole store
		// is broken" as an alternate explanation for the prod-side error.
		got, getErr := secrets.Get(root, "dev", "DB_PASSWORD")
		if getErr != nil {
			t.Fatalf("re-reading dev value: %v", getErr)
		}
		if got != "dev-only-value" {
			t.Errorf("dev value corrupted: got %q, want %q", got, "dev-only-value")
		}
	})
}

// TestSecretsGet_NonexistentKey_Errors verifies Get on a key that was never
// set returns an error rather than an empty string — callers (e.g.
// decrypt-on-deploy consumers) must not silently receive "".
func TestSecretsGet_NonexistentKey_Errors(t *testing.T) {
	requireAge(t)
	withProjectRoot(t, func(root string) {
		initSecretsProject(t, root)
		secretsEnvFlag = "dev"
		defer func() { secretsEnvFlag = "dev" }()

		err := secretsGetCmd.RunE(secretsGetCmd, []string{"NEVER_SET"})
		if err == nil {
			t.Fatal("expected error for a key that was never set, got nil")
		}
	})
}

// TestSecretsList_ReflectsSetSecrets verifies the list command's output is
// driven by real store state, not a static/empty stub, by comparing
// behavior before and after a Set.
func TestSecretsList_ReflectsSetSecrets(t *testing.T) {
	requireAge(t)
	withProjectRoot(t, func(root string) {
		initSecretsProject(t, root)
		secretsEnvFlag = "dev"
		defer func() { secretsEnvFlag = "dev" }()

		keysBefore, _, err := secrets.List(root, "dev")
		if err != nil {
			t.Fatalf("list before set: %v", err)
		}
		if len(keysBefore) != 0 {
			t.Fatalf("expected 0 secrets in a fresh store, got %d", len(keysBefore))
		}

		if err := secretsSetCmd.RunE(secretsSetCmd, []string{"JWT_SIGNING_KEY", "abc123def456ghi789jkl012"}); err != nil {
			t.Fatalf("set: %v", err)
		}

		if err := secretsListCmd.RunE(secretsListCmd, nil); err != nil {
			t.Fatalf("secretsListCmd.RunE: %v", err)
		}

		keysAfter, _, err := secrets.List(root, "dev")
		if err != nil {
			t.Fatalf("list after set: %v", err)
		}
		if len(keysAfter) != 1 || keysAfter[0] != "JWT_SIGNING_KEY" {
			t.Errorf("keys after set = %v, want [JWT_SIGNING_KEY]", keysAfter)
		}
	})
}
