// Package auth — unit tests for the HTTP auth client.
// Tests use httptest.NewServer to mock the auth server; no real network calls.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// serverFunc is a convenience type for httptest handler functions.
type serverFunc func(w http.ResponseWriter, r *http.Request)

// newAuthServer starts a test HTTP server with the given handler and sets
// NSELF_AUTH_SERVER_URL so all client functions hit it.
// The returned cleanup function must be called when the test ends.
func newAuthServer(t *testing.T, h serverFunc) (cleanup func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(h))
	t.Setenv("NSELF_AUTH_SERVER_URL", srv.URL)
	return srv.Close
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ─── DeviceAuthorize ──────────────────────────────────────────────────────────

func TestDeviceAuthorize_ServerError(t *testing.T) {
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":   "service_unavailable",
			"message": "auth server offline",
		})
	})
	defer cleanup()

	_, err := DeviceAuthorize(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDeviceAuthorize_MalformedJSON(t *testing.T) {
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "{not valid json")
	})
	defer cleanup()

	_, err := DeviceAuthorize(context.Background())
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

// ─── RefreshToken ─────────────────────────────────────────────────────────────

func TestRefreshToken_Success(t *testing.T) {
	want := TokenResponse{
		AccessToken: "new.jwt.token",
		Email:       "user@example.com",
		Tier:        "pro",
		ExpiresAt:   "2027-06-01T00:00:00Z",
	}
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/refresh" && r.Method == http.MethodPost {
			// Verify cookie is forwarded
			if r.Header.Get("Cookie") == "" {
				http.Error(w, "missing cookie", http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, want)
			return
		}
		http.NotFound(w, r)
	})
	defer cleanup()

	tok, err := RefreshToken(context.Background(), "old.token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.AccessToken != want.AccessToken {
		t.Errorf("AccessToken: got %q, want %q", tok.AccessToken, want.AccessToken)
	}
}

func TestRefreshToken_Unauthorized(t *testing.T) {
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error":   "invalid_token",
			"message": "session revoked",
		})
	})
	defer cleanup()

	_, err := RefreshToken(context.Background(), "revoked.token")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ─── RevokeSession ────────────────────────────────────────────────────────────

func TestRevokeSession_Success(t *testing.T) {
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/signout" {
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
		http.NotFound(w, r)
	})
	defer cleanup()

	if err := RevokeSession(context.Background(), "valid.token", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRevokeSession_AllSessions(t *testing.T) {
	var gotURL string
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.RequestURI()
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	defer cleanup()

	if err := RevokeSession(context.Background(), "valid.token", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotURL != "/auth/signout?all=true" {
		t.Errorf("expected URL /auth/signout?all=true, got %q", gotURL)
	}
}

func TestRevokeSession_Error(t *testing.T) {
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error":   "invalid_token",
			"message": "not logged in",
		})
	})
	defer cleanup()

	if err := RevokeSession(context.Background(), "bad.token", false); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ─── GetSession ───────────────────────────────────────────────────────────────

func TestGetSession_Success(t *testing.T) {
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/session" {
			resp := AccountInfo{Authenticated: true}
			resp.Account.Email = "user@example.com"
			resp.Account.Tier = "basic"
			resp.Account.EmailVerified = true
			writeJSON(w, http.StatusOK, resp)
			return
		}
		http.NotFound(w, r)
	})
	defer cleanup()

	info, err := GetSession(context.Background(), "valid.token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.Authenticated {
		t.Error("expected Authenticated=true")
	}
	if info.Account.Email != "user@example.com" {
		t.Errorf("Email: got %q", info.Account.Email)
	}
}

func TestGetSession_Unauthorized(t *testing.T) {
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	defer cleanup()

	_, err := GetSession(context.Background(), "expired.token")
	if err == nil {
		t.Fatal("expected ErrNotLoggedIn, got nil")
	}
	if err != ErrNotLoggedIn {
		t.Errorf("expected ErrNotLoggedIn, got %v", err)
	}
}

func TestGetSession_ServerError(t *testing.T) {
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "internal_error",
			"message": "database unreachable",
		})
	})
	defer cleanup()

	_, err := GetSession(context.Background(), "any.token")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ─── GetLicenses ──────────────────────────────────────────────────────────────

func TestGetLicenses_Success(t *testing.T) {
	licenses := []LicenseInfo{
		{
			ID:       "lic-001",
			Product:  "nself",
			Tier:     "basic",
			Bundles:  []string{"nChat", "nClaw"},
			IsActive: true,
		},
	}
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/account/licenses" {
			writeJSON(w, http.StatusOK, map[string]interface{}{"licenses": licenses})
			return
		}
		http.NotFound(w, r)
	})
	defer cleanup()

	got, err := GetLicenses(context.Background(), "valid.token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 license, got %d", len(got))
	}
	if got[0].ID != "lic-001" {
		t.Errorf("ID: got %q", got[0].ID)
	}
}

func TestGetLicenses_Empty(t *testing.T) {
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"licenses": []LicenseInfo{}})
	})
	defer cleanup()

	got, err := GetLicenses(context.Background(), "valid.token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d", len(got))
	}
}

func TestGetLicenses_Error(t *testing.T) {
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error":   "unauthorized",
			"message": "token expired",
		})
	})
	defer cleanup()

	_, err := GetLicenses(context.Background(), "expired.token")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ─── AuthAPIError.Error ───────────────────────────────────────────────────────

func TestAuthAPIError_Error(t *testing.T) {
	e := &AuthAPIError{Code: "test_code", Message: "test message", Status: 422}
	got := e.Error()
	if got == "" {
		t.Error("expected non-empty error string")
	}
	// Verify key fields appear in the message.
	for _, want := range []string{"test_code", "test message", "422"} {
		if !containsString(got, want) {
			t.Errorf("Error() %q missing %q", got, want)
		}
	}
}

// ─── AuthServerURL / CLIAuthBaseURL (env override) ───────────────────────────

func TestAuthServerURL_Default(t *testing.T) {
	t.Setenv("NSELF_AUTH_SERVER_URL", "")
	got := AuthServerURL()
	if got != "https://auth-server.nself.org" {
		t.Errorf("default AuthServerURL: got %q", got)
	}
}

func TestAuthServerURL_Override(t *testing.T) {
	t.Setenv("NSELF_AUTH_SERVER_URL", "http://localhost:9999")
	got := AuthServerURL()
	if got != "http://localhost:9999" {
		t.Errorf("override AuthServerURL: got %q", got)
	}
}

func TestCLIAuthBaseURL_Default(t *testing.T) {
	t.Setenv("NSELF_CLI_AUTH_URL", "")
	got := CLIAuthBaseURL()
	if got != "https://nself.org/auth/cli" {
		t.Errorf("default CLIAuthBaseURL: got %q", got)
	}
}

func TestCLIAuthBaseURL_Override(t *testing.T) {
	t.Setenv("NSELF_CLI_AUTH_URL", "http://localhost:3010/auth/cli")
	got := CLIAuthBaseURL()
	if got != "http://localhost:3010/auth/cli" {
		t.Errorf("override CLIAuthBaseURL: got %q", got)
	}
}

// ─── GetTeamMembers ───────────────────────────────────────────────────────────

func TestGetTeamMembers_Success(t *testing.T) {
	want := []TeamMember{
		{Email: "alice@example.com", Role: "admin"},
		{Email: "bob@example.com", Role: "member"},
	}
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/account/team" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"members": want})
	})
	defer cleanup()

	got, err := GetTeamMembers(context.Background(), "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 members, got %d", len(got))
	}
	if got[0].Email != "alice@example.com" {
		t.Errorf("member[0].Email: got %q", got[0].Email)
	}
}

func TestGetTeamMembers_Error(t *testing.T) {
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized", "message": "bad token"})
	})
	defer cleanup()

	_, err := GetTeamMembers(context.Background(), "bad-tok")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ─── InviteTeamMember ─────────────────────────────────────────────────────────

func TestInviteTeamMember_Success(t *testing.T) {
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/account/team/invite" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
	defer cleanup()

	if err := InviteTeamMember(context.Background(), "tok", "newuser@example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInviteTeamMember_Error(t *testing.T) {
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "already_member", "message": "already in team"})
	})
	defer cleanup()

	if err := InviteTeamMember(context.Background(), "tok", "existing@example.com"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ─── RemoveTeamMember ─────────────────────────────────────────────────────────

func TestRemoveTeamMember_Success(t *testing.T) {
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/account/team/bob@example.com" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	defer cleanup()

	if err := RemoveTeamMember(context.Background(), "tok", "bob@example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRemoveTeamMember_Error(t *testing.T) {
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "member not found"})
	})
	defer cleanup()

	if err := RemoveTeamMember(context.Background(), "tok", "ghost@example.com"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ─── SetTeamMemberRole ────────────────────────────────────────────────────────

func TestSetTeamMemberRole_Success(t *testing.T) {
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/account/team/alice@example.com/role" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	defer cleanup()

	if err := SetTeamMemberRole(context.Background(), "tok", "alice@example.com", "admin"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetTeamMemberRole_Error(t *testing.T) {
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_role", "message": "unknown role"})
	})
	defer cleanup()

	if err := SetTeamMemberRole(context.Background(), "tok", "alice@example.com", "superuser"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ─── ActivateLicense ──────────────────────────────────────────────────────────

func TestActivateLicense_Success(t *testing.T) {
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/account/licenses/lic-123/activate" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	defer cleanup()

	if err := ActivateLicense(context.Background(), "tok", "lic-123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestActivateLicense_Error(t *testing.T) {
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "already_activated", "message": "license in use"})
	})
	defer cleanup()

	if err := ActivateLicense(context.Background(), "tok", "lic-bad"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ─── GetDevices ───────────────────────────────────────────────────────────────

func TestGetDevices_Success(t *testing.T) {
	want := []DeviceEntry{
		{ID: "dev-1", Name: "MacBook Pro", OS: "darwin", IsCurrent: true},
	}
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/account/devices" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"devices": want})
	})
	defer cleanup()

	got, err := GetDevices(context.Background(), "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "dev-1" {
		t.Errorf("unexpected devices: %+v", got)
	}
}

func TestGetDevices_Error(t *testing.T) {
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized", "message": "bad token"})
	})
	defer cleanup()

	_, err := GetDevices(context.Background(), "bad")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ─── RevokeDevice ─────────────────────────────────────────────────────────────

func TestRevokeDevice_Success(t *testing.T) {
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/account/devices/dev-1" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	defer cleanup()

	if err := RevokeDevice(context.Background(), "tok", "dev-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRevokeDevice_Error(t *testing.T) {
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "device not found"})
	})
	defer cleanup()

	if err := RevokeDevice(context.Background(), "tok", "dev-ghost"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ─── TransferLicense ──────────────────────────────────────────────────────────

func TestTransferLicense_Success(t *testing.T) {
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/account/licenses/lic-123/transfer" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	defer cleanup()

	if err := TransferLicense(context.Background(), "tok", "lic-123", "newowner@example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTransferLicense_Error(t *testing.T) {
	cleanup := newAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "transfer_denied", "message": "cannot transfer"})
	})
	defer cleanup()

	if err := TransferLicense(context.Background(), "tok", "lic-bad", "other@example.com"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ─── helpers (test-internal) ──────────────────────────────────────────────────

func containsString(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
