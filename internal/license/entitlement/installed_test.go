package entitlement_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/license"
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

func TestLicenseSetOfflinePreservesAndRejectionClears(t *testing.T) {
	compattest.Set(t, true)
	for _, tc := range []struct {
		name   string
		status int
		body   string
		clear  bool
	}{
		{"unavailable", http.StatusServiceUnavailable, "", false},
		{"server-error", http.StatusInternalServerError, "", false},
		{"rate-limited", http.StatusTooManyRequests, "", false},
		{"bad-request", http.StatusBadRequest, "", false},
		{"not-found", http.StatusNotFound, "", false},
		{"transport", 0, "", false},
		{"timeout", -1, "", false},
		{"invalid", http.StatusUnauthorized, "", true},
		{"forbidden", http.StatusForbidden, `{"valid":false,"error":"License rejected"}`, true},
		{"revoked", http.StatusOK, `{"valid":false,"reason":"revoked"}`, true},
		{"expired", http.StatusOK, `{"valid":false,"reason":"expired"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seedVerifiedCache(t)
			dir, plugins := t.TempDir(), t.TempDir()
			t.Setenv("NSELF_PLUGIN_DIR", plugins)
			installed := filepath.Join(plugins, "fixture")
			if err := os.MkdirAll(installed, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(installed, "plugin.json"), []byte(`{"license":"licensed"}`), 0600); err != nil {
				t.Fatal(err)
			}
			if err := entitlement.Refresh(context.Background(), dir); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(entitlement.Path(dir))
			if err != nil {
				t.Fatal(err)
			}
			pub, priv, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			license.PingKeys = append(license.PingKeys, license.PingKey{ID: "rejection", Public: pub})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.status == -1 {
					select {
					case <-r.Context().Done():
					case <-time.After(100 * time.Millisecond):
					}
					return
				}
				if tc.status == 0 {
					return
				}
				w.Header().Set("X-NSelf-License-Sig", hex.EncodeToString(ed25519.Sign(priv, []byte(tc.body))))
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			if tc.status == 0 {
				server.Close()
			}
			t.Setenv("NSELF_PING_API_URL", server.URL)
			ctx := context.Background()
			if tc.status == -1 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancel()
			}
			err = entitlement.RefreshInstalledForKey(ctx, dir, "synthetic-license-key")
			if tc.clear && errs.ExitCodeFor(err) != 3 {
				t.Fatalf("explicit rejection requires E125: %v", err)
			}
			if !tc.clear && !errors.Is(err, entitlement.ErrValidationUnavailable) {
				t.Fatalf("transient validation must allow a saved-key warning: %v", err)
			}
			after, err := os.ReadFile(entitlement.Path(dir))
			if err != nil {
				t.Fatal(err)
			}
			if !tc.clear && string(after) != string(before) {
				t.Fatal("offline validation destroyed cached runtime entitlement")
			}
			if tc.clear && len(after) != 0 {
				t.Fatal("explicit invalid-key rejection retained runtime entitlement")
			}
		})
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
		{"free-explicit", `{"manifest_version":2,"license":"free","tier":"pro","requiresLicense":true}`, false},
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
