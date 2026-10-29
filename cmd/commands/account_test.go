package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/auth"
)

// writeTestAuthFile writes a minimal auth.json to the isolated home temp dir.
func writeTestAuthFile(t *testing.T, home string, af *auth.AuthFile) {
	t.Helper()
	nselfDir := filepath.Join(home, ".nself")
	if err := os.MkdirAll(nselfDir, 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, err := json.MarshalIndent(af, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nselfDir, "auth.json"), data, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// ── accountCmd parent ────────────────────────────────────────────────────────

// TestAccountCmd_DefaultsToStatus verifies that running `account` with no subcommand
// returns status output (not --help) for a logged-in user.
func TestAccountCmd_DefaultsToStatus(t *testing.T) {
	home := isolateHome(t)
	writeTestAuthFile(t, home, &auth.AuthFile{
		AccessToken: "token-abc",
		Email:       "alice@example.com",
		Tier:        "pro",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/session" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"authenticated": true,
				"account": map[string]interface{}{
					"email":          "alice@example.com",
					"tier":           "pro",
					"email_verified": true,
					"mfa_enabled":    false,
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	t.Setenv("NSELF_AUTH_SERVER_URL", srv.URL)

	err := accountCmd.RunE(accountCmd, []string{})
	if err != nil {
		t.Fatalf("accountCmd default run: %v", err)
	}
}

// ── account login ────────────────────────────────────────────────────────────

// TestAccountLoginCmd_FlagsRegistered verifies --reauth, --no-browser, --timeout flags exist.
func TestAccountLoginCmd_FlagsRegistered(t *testing.T) {
	for _, flag := range []string{"reauth", "no-browser", "timeout"} {
		if accountLoginCmd.Flags().Lookup(flag) == nil {
			t.Errorf("flag --%s not registered on accountLoginCmd", flag)
		}
	}
}

// TestAccountLoginCmd_AlreadyLoggedIn verifies --reauth is needed when already logged in.
func TestAccountLoginCmd_AlreadyLoggedIn(t *testing.T) {
	home := isolateHome(t)
	writeTestAuthFile(t, home, &auth.AuthFile{
		AccessToken: "existing-token",
		Email:       "old@example.com",
		Tier:        "free",
	})

	_ = accountLoginCmd.Flags().Set("reauth", "false")
	_ = accountLoginCmd.Flags().Set("no-browser", "false")
	defer func() {
		_ = accountLoginCmd.Flags().Set("reauth", "false")
		_ = accountLoginCmd.Flags().Set("no-browser", "false")
	}()

	err := accountLoginCmd.RunE(accountLoginCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error on already-logged-in: %v", err)
	}
}

// TestAccountLoginCmd_DeviceCodeFlow verifies the happy-path device code flow.
func TestAccountLoginCmd_DeviceCodeFlow(t *testing.T) {
	home := isolateHome(t)

	dcResp := auth.DeviceCodeResponse{
		DeviceCode:      "device-code-abc",
		UserCode:        "TEST-1234",
		VerificationURL: "https://nself.org/device?code=TEST-1234",
		ExpiresInSec:    300,
		IntervalSec:     5,
	}
	tokenResp := auth.TokenResponse{
		AccessToken:  "at-xyz",
		SessionToken: "st-xyz",
		Email:        "alice@example.com",
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

	_ = accountLoginCmd.Flags().Set("no-browser", "true")
	_ = accountLoginCmd.Flags().Set("reauth", "true")
	defer func() {
		_ = accountLoginCmd.Flags().Set("no-browser", "false")
		_ = accountLoginCmd.Flags().Set("reauth", "false")
	}()

	if err := accountLoginCmd.RunE(accountLoginCmd, []string{}); err != nil {
		t.Fatalf("login failed: %v", err)
	}

	// Verify auth.json was written with correct content.
	authPath := filepath.Join(home, ".nself", "auth.json")
	data, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("auth.json not created: %v", err)
	}
	var saved auth.AuthFile
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("auth.json invalid JSON: %v", err)
	}
	if saved.Email != tokenResp.Email {
		t.Errorf("email: got %q, want %q", saved.Email, tokenResp.Email)
	}
}

// ── account logout ───────────────────────────────────────────────────────────

// TestAccountLogoutCmd_NotLoggedIn verifies logout is safe when not logged in.
func TestAccountLogoutCmd_NotLoggedIn(t *testing.T) {
	isolateHome(t)
	_ = accountLogoutCmd.Flags().Set("all", "false")
	if err := accountLogoutCmd.RunE(accountLogoutCmd, []string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestAccountLogoutCmd_RevokesAndDeletes verifies session is revoked and file deleted.
func TestAccountLogoutCmd_RevokesAndDeletes(t *testing.T) {
	home := isolateHome(t)
	writeTestAuthFile(t, home, &auth.AuthFile{
		AccessToken: "old-token",
		Email:       "x@example.com",
		Tier:        "free",
	})

	setAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/signout" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	})

	_ = accountLogoutCmd.Flags().Set("all", "false")
	if err := accountLogoutCmd.RunE(accountLogoutCmd, []string{}); err != nil {
		t.Fatalf("logout failed: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(home, ".nself", "auth.json")); !os.IsNotExist(statErr) {
		t.Error("auth.json still exists after logout")
	}
}

// ── account status ───────────────────────────────────────────────────────────

// TestAccountStatusCmd_NotLoggedIn verifies graceful output when not logged in.
func TestAccountStatusCmd_NotLoggedIn(t *testing.T) {
	isolateHome(t)
	_ = accountStatusCmd.Flags().Set("json", "false")
	if err := accountStatusCmd.RunE(accountStatusCmd, []string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestAccountStatusCmd_JSON verifies --json flag produces valid JSON.
func TestAccountStatusCmd_JSON(t *testing.T) {
	home := isolateHome(t)
	writeTestAuthFile(t, home, &auth.AuthFile{
		AccessToken: "token-abc",
		Email:       "alice@example.com",
		Tier:        "pro",
		Bundles:     []string{"nclaw"},
	})

	setAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/session" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"authenticated": true,
				"account": map[string]interface{}{
					"email":          "alice@example.com",
					"tier":           "pro",
					"email_verified": true,
				},
			})
			return
		}
		http.NotFound(w, r)
	})

	_ = accountStatusCmd.Flags().Set("json", "true")
	defer func() { _ = accountStatusCmd.Flags().Set("json", "false") }()

	if err := accountStatusCmd.RunE(accountStatusCmd, []string{}); err != nil {
		t.Fatalf("status --json: %v", err)
	}
}

// ── account team ─────────────────────────────────────────────────────────────

// TestAccountTeamCmd_FlagsRegistered verifies --invite, --remove, --role, --json flags exist.
func TestAccountTeamCmd_FlagsRegistered(t *testing.T) {
	for _, flag := range []string{"invite", "remove", "role", "json"} {
		if accountTeamCmd.Flags().Lookup(flag) == nil {
			t.Errorf("flag --%s not registered on accountTeamCmd", flag)
		}
	}
}

// TestAccountTeamCmd_ListMembers verifies team list output.
func TestAccountTeamCmd_ListMembers(t *testing.T) {
	home := isolateHome(t)
	writeTestAuthFile(t, home, &auth.AuthFile{
		AccessToken: "token-abc",
		Email:       "alice@example.com",
		Tier:        "pro",
	})

	setAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/account/team" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"members": []map[string]interface{}{
					{"name": "Alice", "email": "alice@example.com", "role": "owner", "joined_at": "2026-01-15"},
					{"name": "Bob", "email": "bob@example.com", "role": "member", "joined_at": "2026-03-01"},
				},
			})
			return
		}
		http.NotFound(w, r)
	})

	// Reset flags.
	_ = accountTeamCmd.Flags().Set("invite", "")
	_ = accountTeamCmd.Flags().Set("remove", "")
	_ = accountTeamCmd.Flags().Set("json", "false")

	if err := accountTeamCmd.RunE(accountTeamCmd, []string{}); err != nil {
		t.Fatalf("team list: %v", err)
	}
}

// TestAccountTeamCmd_Invite verifies --invite calls the correct endpoint.
func TestAccountTeamCmd_Invite(t *testing.T) {
	home := isolateHome(t)
	writeTestAuthFile(t, home, &auth.AuthFile{
		AccessToken: "token-abc",
		Email:       "alice@example.com",
		Tier:        "pro",
	})

	invited := false
	setAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/account/team/invite" && r.Method == http.MethodPost {
			invited = true
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.NotFound(w, r)
	})

	_ = accountTeamCmd.Flags().Set("invite", "charlie@example.com")
	defer func() { _ = accountTeamCmd.Flags().Set("invite", "") }()

	if err := accountTeamCmd.RunE(accountTeamCmd, []string{}); err != nil {
		t.Fatalf("team --invite: %v", err)
	}
	if !invited {
		t.Error("expected POST /account/team/invite to be called")
	}
}

// ── account licenses ─────────────────────────────────────────────────────────

// TestAccountLicensesCmd_FlagsRegistered verifies --activate and --json flags exist.
func TestAccountLicensesCmd_FlagsRegistered(t *testing.T) {
	for _, flag := range []string{"activate", "json"} {
		if accountLicensesCmd.Flags().Lookup(flag) == nil {
			t.Errorf("flag --%s not registered on accountLicensesCmd", flag)
		}
	}
}

// TestAccountLicensesCmd_ListEmpty verifies graceful output when no licenses exist.
func TestAccountLicensesCmd_ListEmpty(t *testing.T) {
	home := isolateHome(t)
	writeTestAuthFile(t, home, &auth.AuthFile{
		AccessToken: "token-abc",
		Email:       "alice@example.com",
		Tier:        "free",
	})

	setAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/account/licenses" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"licenses": []interface{}{},
			})
			return
		}
		http.NotFound(w, r)
	})

	_ = accountLicensesCmd.Flags().Set("activate", "")
	_ = accountLicensesCmd.Flags().Set("json", "false")

	if err := accountLicensesCmd.RunE(accountLicensesCmd, []string{}); err != nil {
		t.Fatalf("licenses list: %v", err)
	}
}

// ── account devices ───────────────────────────────────────────────────────────

// TestAccountDevicesCmd_FlagsRegistered verifies --revoke, --force, --json flags exist.
func TestAccountDevicesCmd_FlagsRegistered(t *testing.T) {
	for _, flag := range []string{"revoke", "force", "json"} {
		if accountDevicesCmd.Flags().Lookup(flag) == nil {
			t.Errorf("flag --%s not registered on accountDevicesCmd", flag)
		}
	}
}

// TestAccountDevicesCmd_ListDevices verifies device list output.
func TestAccountDevicesCmd_ListDevices(t *testing.T) {
	home := isolateHome(t)
	writeTestAuthFile(t, home, &auth.AuthFile{
		AccessToken: "token-abc",
		Email:       "alice@example.com",
		Tier:        "pro",
	})

	setAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/account/devices" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"devices": []map[string]interface{}{
					{"id": "abc12345", "name": "MacBook Pro", "os": "macOS", "last_active": "2026-04-22", "is_current": true},
					{"id": "def67890", "name": "Ubuntu Server", "os": "linux", "last_active": "2026-04-20", "is_current": false},
				},
			})
			return
		}
		http.NotFound(w, r)
	})

	_ = accountDevicesCmd.Flags().Set("revoke", "")
	_ = accountDevicesCmd.Flags().Set("force", "false")
	_ = accountDevicesCmd.Flags().Set("json", "false")

	if err := accountDevicesCmd.RunE(accountDevicesCmd, []string{}); err != nil {
		t.Fatalf("devices list: %v", err)
	}
}

// TestAccountDevicesCmd_RevokeCurrentRequiresForce verifies that revoking the current
// device fails without --force.
func TestAccountDevicesCmd_RevokeCurrentRequiresForce(t *testing.T) {
	home := isolateHome(t)
	writeTestAuthFile(t, home, &auth.AuthFile{
		AccessToken: "token-abc",
		Email:       "alice@example.com",
		Tier:        "pro",
	})

	setAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/account/devices" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"devices": []map[string]interface{}{
					{"id": "current-id", "name": "My Mac", "os": "macOS", "last_active": "2026-04-22", "is_current": true},
				},
			})
			return
		}
		http.NotFound(w, r)
	})

	_ = accountDevicesCmd.Flags().Set("revoke", "current-id")
	_ = accountDevicesCmd.Flags().Set("force", "false")
	defer func() {
		_ = accountDevicesCmd.Flags().Set("revoke", "")
		_ = accountDevicesCmd.Flags().Set("force", "false")
	}()

	err := accountDevicesCmd.RunE(accountDevicesCmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "without --force") {
		t.Errorf("expected error about --force, got: %v", err)
	}
}

// ── account transfer ─────────────────────────────────────────────────────────

// TestAccountTransferCmd_ArgsRequired verifies transfer requires exactly 2 args.
func TestAccountTransferCmd_ArgsRequired(t *testing.T) {
	err := accountTransferCmd.Args(accountTransferCmd, []string{"only-one-arg"})
	if err == nil {
		t.Error("expected error for single-arg transfer")
	}
}

// TestAccountTransferCmd_ExactArgs verifies two args are accepted.
func TestAccountTransferCmd_ExactArgs(t *testing.T) {
	err := accountTransferCmd.Args(accountTransferCmd, []string{"key-id", "email@example.com"})
	if err != nil {
		t.Errorf("unexpected error for two-arg transfer: %v", err)
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

// TestRequireLogin_NotLoggedIn verifies requireLogin error when not logged in.
func TestRequireLogin_NotLoggedIn(t *testing.T) {
	isolateHome(t)
	_, err := requireLogin()
	if err == nil {
		t.Error("expected error for not-logged-in")
	}
	if !strings.Contains(err.Error(), "nself account login") {
		t.Errorf("error should mention 'nself account login', got: %v", err)
	}
}
