package commands

// secrets_edit_rotate_test.go — Unit tests for `nself secrets rotate` and
// `retire` (secrets_edit_rotate.go). P6-E11-W2-S3-T18: security command
// test floor (was 0% direct coverage).
//
// Security property under test: rotation must actually INVALIDATE the old
// secret value, not just write a new one alongside it under a different
// name. A rotate command that "succeeds" while the old value remains
// readable under its original key gives operators false confidence that a
// compromised credential has been replaced when it has not.
//   - Plain rotate: Get(key) after rotate must differ from the pre-rotate
//     value (old value gone from the primary slot).
//   - Dual-window rotate: Get(key) must return the NEW value immediately
//     (so consumers cut over right away), while the OLD value is still
//     reachable only via the explicit _PREVIOUS slot during the overlap
//     window — and retire must then remove that _PREVIOUS slot entirely,
//     so the old value becomes unreachable through any command.
// Inputs: temp project roots initialized via secretsInitCmd (see
// secrets_core_test.go's initSecretsProject/withProjectRoot/requireAge).
// Outputs: rotated/retired store state.
// Constraints: requires the `age`/`age-keygen` binaries; skips cleanly
// when unavailable.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/secrets"
)

// TestSecretsRotate_InvalidatesOldValue verifies plain (non-dual-window)
// rotation actually replaces the value under the original key, for a
// secret type rotate CAN auto-generate a replacement for (a password).
func TestSecretsRotate_InvalidatesOldValue(t *testing.T) {
	requireAge(t)
	withProjectRoot(t, func(root string) {
		initSecretsProject(t, root)
		secretsEnvFlag = "dev"
		defer func() { secretsEnvFlag = "dev" }()

		if err := secretsSetCmd.RunE(secretsSetCmd, []string{"SERVICE_PASSWORD", "old-compromised-value-v1"}); err != nil {
			t.Fatalf("set: %v", err)
		}

		_ = secretsRotateCmd.Flags().Set("dual-window", "false")
		if err := secretsRotateCmd.RunE(secretsRotateCmd, []string{"SERVICE_PASSWORD"}); err != nil {
			t.Fatalf("rotate: %v", err)
		}

		newValue, err := secrets.Get(root, "dev", "SERVICE_PASSWORD")
		if err != nil {
			t.Fatalf("get after rotate: %v", err)
		}
		if newValue == "old-compromised-value-v1" {
			t.Fatal("SERVICE_PASSWORD still holds the pre-rotation value — rotation did not invalidate it")
		}
	})
}

// TestSecretsRotate_ManualRotationType_PreservesOldValueOnError verifies the
// fix for the 2026-09-11/12 incident class: a secret type rotate cannot
// auto-generate a replacement for (API keys/tokens, which must be rotated
// through the provider's dashboard) must error out AND leave the existing
// value completely untouched — never silently overwrite it with an empty
// string, which is exactly as destructive as the incident's "${VAR}"
// reference collapsing to empty on capture.
func TestSecretsRotate_ManualRotationType_PreservesOldValueOnError(t *testing.T) {
	requireAge(t)
	withProjectRoot(t, func(root string) {
		initSecretsProject(t, root)
		secretsEnvFlag = "dev"
		defer func() { secretsEnvFlag = "dev" }()

		if err := secretsSetCmd.RunE(secretsSetCmd, []string{"API_TOKEN", "old-compromised-value-v1"}); err != nil {
			t.Fatalf("set: %v", err)
		}

		_ = secretsRotateCmd.Flags().Set("dual-window", "false")
		if err := secretsRotateCmd.RunE(secretsRotateCmd, []string{"API_TOKEN"}); err == nil {
			t.Fatal("expected rotate on a manual-rotation-only key (API_TOKEN) to error, got nil")
		}

		stillThere, err := secrets.Get(root, "dev", "API_TOKEN")
		if err != nil {
			t.Fatalf("get after failed rotate: %v", err)
		}
		if stillThere != "old-compromised-value-v1" {
			t.Fatalf("API_TOKEN value changed after a failed rotate — got %q, want the untouched original", stillThere)
		}
	})
}

// TestSecretsRotateDualWindow_OldValueOnlyReachableViaPrevious verifies the
// dual-window property: immediately after rotation the base key returns the
// NEW value, and the OLD value is reachable ONLY via the _PREVIOUS slot —
// never under the base key or any other name.
func TestSecretsRotateDualWindow_OldValueOnlyReachableViaPrevious(t *testing.T) {
	requireAge(t)
	withProjectRoot(t, func(root string) {
		initSecretsProject(t, root)
		secretsEnvFlag = "dev"
		defer func() { secretsEnvFlag = "dev" }()

		if err := secretsSetCmd.RunE(secretsSetCmd, []string{"SESSION_SECRET", "old-session-secret-original-v1"}); err != nil {
			t.Fatalf("set: %v", err)
		}

		_ = secretsRotateCmd.Flags().Set("dual-window", "true")
		defer func() { _ = secretsRotateCmd.Flags().Set("dual-window", "false") }()
		if err := secretsRotateCmd.RunE(secretsRotateCmd, []string{"SESSION_SECRET"}); err != nil {
			t.Fatalf("dual-window rotate: %v", err)
		}

		base, err := secrets.Get(root, "dev", "SESSION_SECRET")
		if err != nil {
			t.Fatalf("get base after dual-window rotate: %v", err)
		}
		if base == "old-session-secret-original-v1" {
			t.Fatal("base key still returns the OLD value after dual-window rotate — new value never took effect")
		}

		prev, err := secrets.Get(root, "dev", "SESSION_SECRET_PREVIOUS")
		if err != nil {
			t.Fatalf("get _PREVIOUS: %v", err)
		}
		if prev != "old-session-secret-original-v1" {
			t.Errorf("SESSION_SECRET_PREVIOUS = %q, want the pre-rotation value %q", prev, "old-session-secret-original-v1")
		}

		// Retire the old key window: _PREVIOUS must become entirely unreachable.
		if err := secretsRetireCmd.RunE(secretsRetireCmd, []string{"SESSION_SECRET"}); err != nil {
			t.Fatalf("retire: %v", err)
		}
		if _, err := secrets.Get(root, "dev", "SESSION_SECRET_PREVIOUS"); err == nil {
			t.Fatal("SESSION_SECRET_PREVIOUS is still readable after retire — old value was never revoked")
		}

		// The base (now-current) value must survive retirement unchanged.
		afterRetire, err := secrets.Get(root, "dev", "SESSION_SECRET")
		if err != nil {
			t.Fatalf("get base after retire: %v", err)
		}
		if afterRetire != base {
			t.Errorf("retire changed the current value: got %q, want %q", afterRetire, base)
		}
	})
}

// TestSecretsRotate_UnknownKey_Errors verifies rotating a key that was never
// set fails loudly instead of silently creating a bogus rotated entry. This
// path needs no `age` binary: loadStore returns an empty store when the
// file is absent, so the "not found" lookup happens before any encryption.
func TestSecretsRotate_UnknownKey_Errors(t *testing.T) {
	withProjectRoot(t, func(root string) {
		secretsEnvFlag = "dev"
		defer func() { secretsEnvFlag = "dev" }()
		_ = secretsRotateCmd.Flags().Set("dual-window", "false")

		err := secretsRotateCmd.RunE(secretsRotateCmd, []string{"NEVER_SET_KEY"})
		if err == nil {
			t.Fatal("expected error rotating a key that was never set, got nil")
		}
	})
}

// TestSecretsEdit_RoundTrip_AppliesEditorChanges verifies the real property
// of `nself secrets edit`: whatever the $EDITOR process writes back into the
// decrypted temp file is actually persisted into the encrypted store — a
// regression that decrypted to the temp file but never re-read it (or
// re-read the wrong path) would silently discard every edit while still
// printing "Secrets updated."
func TestSecretsEdit_RoundTrip_AppliesEditorChanges(t *testing.T) {
	requireAge(t)
	withProjectRoot(t, func(root string) {
		initSecretsProject(t, root)
		secretsEnvFlag = "dev"
		defer func() { secretsEnvFlag = "dev" }()

		if err := secretsSetCmd.RunE(secretsSetCmd, []string{"FOO", "original-value"}); err != nil {
			t.Fatalf("set: %v", err)
		}

		// A fake $EDITOR that overwrites whatever temp file it's given with a
		// new value, simulating a human editing the decrypted file and saving.
		editorScript := filepath.Join(root, "fake-editor.sh")
		script := "#!/bin/sh\necho \"FOO=changed-by-editor\" > \"$1\"\n"
		if err := os.WriteFile(editorScript, []byte(script), 0o755); err != nil {
			t.Fatalf("write fake editor: %v", err)
		}
		t.Setenv("EDITOR", editorScript)

		if err := secretsEditCmd.RunE(secretsEditCmd, nil); err != nil {
			t.Fatalf("secretsEditCmd.RunE: %v", err)
		}

		got, err := secrets.Get(root, "dev", "FOO")
		if err != nil {
			t.Fatalf("get after edit: %v", err)
		}
		if got != "changed-by-editor" {
			t.Fatalf("FOO = %q after edit, want %q — editor changes were not persisted", got, "changed-by-editor")
		}
	})
}

// TestSecretsRetire_NoWindow_Errors verifies retire fails when there is no
// _PREVIOUS to retire, rather than silently succeeding — a silent success
// here would mask an operator's mistaken belief that an overlap window is
// still active and safe to close.
func TestSecretsRetire_NoWindow_Errors(t *testing.T) {
	withProjectRoot(t, func(root string) {
		secretsEnvFlag = "dev"
		defer func() { secretsEnvFlag = "dev" }()

		err := secretsRetireCmd.RunE(secretsRetireCmd, []string{"NEVER_ROTATED"})
		if err == nil {
			t.Fatal("expected error retiring a key with no _PREVIOUS window, got nil")
		}
	})
}
