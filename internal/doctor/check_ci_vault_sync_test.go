package doctor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
)

// CI-VAULT-SYNC-01 reads ~/.claude/vault.env, which cannot exist on a CI runner,
// so it returned CRITICAL on every single run — a gate that could never pass.
func TestCIVaultSync_SkipsOnCIWithoutVault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("CI", "true")

	got := CheckCIVaultSync(dir)
	if got.Status != "skip" {
		t.Fatalf("status = %q, want \"skip\" (a CI runner has no vault.env by design)", got.Status)
	}
}

// Guards the other direction so this cannot become a gate that never fails:
// when a vault file is present the check still runs, even on CI.
func TestCIVaultSync_StillChecksWhenVaultPresent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("GITHUB_ACTIONS", "true")

	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	// Vault exists but contains none of the critical PATs.
	vault := filepath.Join(dir, ".claude", "vault.env")
	if err := os.WriteFile(vault, []byte("UNRELATED=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := CheckCIVaultSync(dir); got.Status != "fail" {
		t.Fatalf("status = %q, want \"fail\" — a present-but-empty vault must still fail", got.Status)
	}
}

// TestVaultSyncV15Both (P7-CI-45): the deep doctor runs CI-VAULT-SYNC-01 in v1.4
// and no longer runs it in v1.5, because it reads ~/.claude/vault.env, a private
// operator file that product code must not depend on. CI-TOKEN-01 stays in both.
func TestVaultSyncV15Both(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("HOME", dir)
		t.Setenv("USERPROFILE", dir)
		t.Setenv("NSELF_DOGFOOD", "")
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // every probe fails fast; the check list is what is under test

		var vault, token int
		for _, r := range DeepChecks(ctx, dir, false) {
			switch r.Name {
			case "CI-VAULT-SYNC-01":
				vault++
			case "CI-TOKEN-01":
				token++
			}
		}
		wantVault := 1
		if compat.V15() {
			wantVault = 0
		}
		if vault != wantVault {
			t.Errorf("CI-VAULT-SYNC-01 entries = %d, want %d", vault, wantVault)
		}
		if token != 1 {
			t.Errorf("CI-TOKEN-01 entries = %d, want 1 in both modes", token)
		}
	})
}
