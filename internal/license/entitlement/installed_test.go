package entitlement_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/license/entitlement"
)

func TestRefreshRejectsAnotherConfiguredLicence(t *testing.T) {
	compattest.Set(t, true)
	seedVerifiedCache(t)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "synthetic-other-license-key")
	dir := t.TempDir()
	if err := entitlement.Refresh(context.Background(), dir); errs.ExitCodeFor(err) != 3 {
		t.Fatalf("another configured licence must reject cached JWT: %v", err)
	}
	data, err := os.ReadFile(entitlement.Path(dir))
	if err != nil || len(data) != 0 {
		t.Fatalf("mismatched licence must leave empty runtime file: %v", err)
	}
}

func TestRefreshInstalledFreeAndDisabled(t *testing.T) {
	compattest.Set(t, true)
	for _, tc := range []struct {
		name     string
		manifest string
		disabled bool
	}{
		{"free", `{"license":"free"}`, false},
		{"free-explicit", `{"license":"free","tier":"pro","requiresLicense":true}`, false},
		{"disabled", `{"license":"licensed"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, plugins := t.TempDir(), t.TempDir()
			t.Setenv("NSELF_PLUGIN_DIR", plugins)
			installed := filepath.Join(plugins, "fixture")
			if err := os.MkdirAll(installed, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(installed, "plugin.json"), []byte(tc.manifest), 0600); err != nil {
				t.Fatal(err)
			}
			if tc.disabled {
				if err := os.WriteFile(filepath.Join(installed, ".disabled"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			// A nonempty new key must not trigger online validation for these cases.
			if err := entitlement.RefreshInstalledForKey(context.Background(), dir, "synthetic-key"); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(entitlement.Path(dir)); !os.IsNotExist(err) {
				t.Fatalf("free/disabled plugin must not create entitlement state: %v", err)
			}
		})
	}
}
