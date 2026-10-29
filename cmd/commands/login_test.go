package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/nself-org/cli/internal/auth"
)

// setAuthServer overrides NSELF_AUTH_SERVER_URL for the test and returns cleanup.
func setAuthServer(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Setenv("NSELF_AUTH_SERVER_URL", srv.URL)
	t.Cleanup(srv.Close)
}

// isolateHome redirects HOME (and USERPROFILE on Windows) to a temp dir so
// auth.json writes are isolated regardless of platform.
func isolateHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	if runtime.GOOS == "windows" {
		// os.UserHomeDir() reads USERPROFILE on Windows, not HOME.
		t.Setenv("USERPROFILE", tmp)
	}
	return tmp
}

// TestLoginCmd_NoBrowserFlag verifies --no-browser flag is registered and parseable.
func TestLoginCmd_NoBrowserFlag(t *testing.T) {
	if loginCmd.Flags().Lookup("no-browser") == nil {
		t.Error("--no-browser flag not registered on loginCmd")
	}
}

// TestLoginCmd_ForceFlag verifies --force flag is registered.
func TestLoginCmd_ForceFlag(t *testing.T) {
	if loginCmd.Flags().Lookup("force") == nil {
		t.Error("--force flag not registered on loginCmd")
	}
}

// TestLoginCmd_AlreadyLoggedIn verifies that login without --force short-circuits
// when an auth.json with a valid token already exists.
func TestLoginCmd_AlreadyLoggedIn(t *testing.T) {
	home := isolateHome(t)

	// Write a pre-existing auth.json.
	af := &auth.AuthFile{
		AccessToken: "existing-token",
		Email:       "old@example.com",
		Tier:        "basic",
	}
	nselfDir := filepath.Join(home, ".nself")
	_ = os.MkdirAll(nselfDir, 0700)
	data, _ := json.MarshalIndent(af, "", "  ")
	_ = os.WriteFile(filepath.Join(nselfDir, "auth.json"), data, 0600)

	// Capture stdout.
	var buf bytes.Buffer
	loginCmd.SetOut(&buf)
	loginCmd.SetErr(&buf)

	// Run without --force.
	loginCmd.SetArgs([]string{})
	// Reset flags to defaults before run.
	_ = loginCmd.Flags().Set("force", "false")
	_ = loginCmd.Flags().Set("no-browser", "false")

	err := loginCmd.RunE(loginCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestLoginCmd_DeviceCodeFlow verifies the happy-path device code flow.
func TestLoginCmd_DeviceCodeFlow(t *testing.T) {
	home := isolateHome(t)

	// Reset loginCmd's ctx. If a prior test (e.g., the error harness) invoked
	// RootCmd.ExecuteContext with a bounded/cancelled context, cobra retains
	// that ctx on the parent + child tree. Explicitly reset to a fresh
	// Background so login's internal 6-minute timeout is the only deadline.
	loginCmd.SetContext(context.Background())

	dcResp := auth.DeviceCodeResponse{
		DeviceCode:      "test-device-code",
		UserCode:        "TEST-CODE",
		VerificationURL: "https://nself.org/device?code=TEST-CODE",
		ExpiresInSec:    300,
		IntervalSec:     5,
	}
	tokenResp := auth.TokenResponse{
		AccessToken:  "at-abc123",
		SessionToken: "st-xyz789",
		Email:        "user@example.com",
		Tier:         "pro",
		Bundles:      []string{"nclaw"},
	}

	setAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth/device-code":
			_ = json.NewEncoder(w).Encode(dcResp)
		case "/auth/device-code/token":
			// auth_server answers an approved poll with the device token only.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": tokenResp.AccessToken,
				"token_type":   "Bearer",
				"expires_in":   7776000,
			})
		case "/auth/session":
			// Identity comes from GET /auth/session, not from the poll reply.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"authenticated": true,
				"account": map[string]any{
					"id":    "acct-1",
					"email": tokenResp.Email,
					"tier":  tokenResp.Tier,
				},
			})
		default:
			http.NotFound(w, r)
		}
	})

	// Run login with --no-browser so we don't try to open a real browser.
	_ = loginCmd.Flags().Set("no-browser", "true")
	_ = loginCmd.Flags().Set("force", "true")
	defer func() {
		_ = loginCmd.Flags().Set("no-browser", "false")
		_ = loginCmd.Flags().Set("force", "false")
	}()

	err := loginCmd.RunE(loginCmd, []string{})
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	// Verify auth.json was written.
	authPath := filepath.Join(home, ".nself", "auth.json")
	data, readErr := os.ReadFile(authPath)
	if readErr != nil {
		t.Fatalf("auth.json not created: %v", readErr)
	}

	var saved auth.AuthFile
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("auth.json is not valid JSON: %v", err)
	}
	if saved.Email != tokenResp.Email {
		t.Errorf("saved email: got %q, want %q", saved.Email, tokenResp.Email)
	}
	if saved.Tier != tokenResp.Tier {
		t.Errorf("saved tier: got %q, want %q", saved.Tier, tokenResp.Tier)
	}

	// Verify 0600 permissions (Unix only — Windows does not honour permission bits).
	if runtime.GOOS != "windows" {
		fi, statErr := os.Stat(authPath)
		if statErr != nil {
			t.Fatalf("stat auth.json: %v", statErr)
		}
		if fi.Mode().Perm() != 0600 {
			t.Errorf("auth.json permissions: got %o, want 0600", fi.Mode().Perm())
		}
	}
}

// TestLogoutCmd_NotLoggedIn verifies logout returns nil when not logged in.
func TestLogoutCmd_NotLoggedIn(t *testing.T) {
	isolateHome(t)

	err := logoutCmd.RunE(logoutCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No error is the only contract — "Not logged in." is printed to stdout.
}

// TestLogoutCmd_RevokesAndDeletesFile verifies session revocation and file deletion.
func TestLogoutCmd_RevokesAndDeletesFile(t *testing.T) {
	home := isolateHome(t)

	// Write existing credentials.
	af := &auth.AuthFile{AccessToken: "old-token", Email: "x@example.com", Tier: "free"}
	nselfDir := filepath.Join(home, ".nself")
	_ = os.MkdirAll(nselfDir, 0700)
	data, _ := json.MarshalIndent(af, "", "  ")
	_ = os.WriteFile(filepath.Join(nselfDir, "auth.json"), data, 0600)

	setAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/signout" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	})

	_ = logoutCmd.Flags().Set("all", "false")
	err := logoutCmd.RunE(logoutCmd, []string{})
	if err != nil {
		t.Fatalf("logout failed: %v", err)
	}

	// auth.json must be deleted.
	if _, statErr := os.Stat(filepath.Join(nselfDir, "auth.json")); !os.IsNotExist(statErr) {
		t.Error("auth.json still exists after logout")
	}
}

// TestAccountShowCmd_NotLoggedIn verifies account show errors gracefully when not logged in.
func TestAccountShowCmd_NotLoggedIn(t *testing.T) {
	isolateHome(t)

	err := accountShowCmd.RunE(accountShowCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
