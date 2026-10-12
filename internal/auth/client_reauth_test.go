package auth

// Purpose: D-0480. Every token-bearing client call maps a 401 to ErrNotLoggedIn
// and keeps a 403 as an *AuthAPIError (tier denial). Fake token, httptest stub.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTokenCalls_401IsErrNotLoggedIn_403IsAPIError(t *testing.T) {
	calls := map[string]func(ctx context.Context) error{
		"GetSession":        func(ctx context.Context) error { _, e := GetSession(ctx, "fake"); return e },
		"RefreshToken":      func(ctx context.Context) error { _, e := RefreshToken(ctx, "fake"); return e },
		"RevokeSession":     func(ctx context.Context) error { return RevokeSession(ctx, "fake", false) },
		"GetLicenses":       func(ctx context.Context) error { _, e := GetLicenses(ctx, "fake"); return e },
		"GetDevices":        func(ctx context.Context) error { _, e := GetDevices(ctx, "fake"); return e },
		"RevokeDevice":      func(ctx context.Context) error { return RevokeDevice(ctx, "fake", "d1") },
		"TransferLicense":   func(ctx context.Context) error { return TransferLicense(ctx, "fake", "l1", "b@example.com") },
		"GetTeamMembers":    func(ctx context.Context) error { _, e := GetTeamMembers(ctx, "fake"); return e },
		"InviteTeamMember":  func(ctx context.Context) error { return InviteTeamMember(ctx, "fake", "b@example.com") },
		"RemoveTeamMember":  func(ctx context.Context) error { return RemoveTeamMember(ctx, "fake", "b@example.com") },
		"SetTeamMemberRole": func(ctx context.Context) error { return SetTeamMemberRole(ctx, "fake", "b@example.com", "admin") },
		"ActivateLicense":   func(ctx context.Context) error { return ActivateLicense(ctx, "fake", "l1") },
	}
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"x","message":"y"}`))
		}))
		t.Setenv("NSELF_AUTH_SERVER_URL", srv.URL)
		for name, call := range calls {
			err := call(context.Background())
			var apiErr *AuthAPIError
			isAPI := errors.As(err, &apiErr)
			switch status {
			case http.StatusUnauthorized:
				if err != ErrNotLoggedIn {
					t.Errorf("%s: 401 gave %v, want ErrNotLoggedIn", name, err)
				}
			case http.StatusForbidden:
				if !isAPI || apiErr.Status != 403 {
					t.Errorf("%s: 403 gave %v, want *AuthAPIError status 403", name, err)
				}
			}
		}
		srv.Close()
	}
}

// TestDeviceFlow_401StaysAPIError: the pre-login device flow carries no token,
// so a 401 there is not "token rejected".
func TestDeviceFlow_401StaysAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"x","message":"y"}`))
	}))
	defer srv.Close()
	t.Setenv("NSELF_AUTH_SERVER_URL", srv.URL)
	if _, err := PollToken(context.Background(), "dc"); err == nil || err == ErrNotLoggedIn {
		t.Errorf("PollToken 401 = %v, want an API error", err)
	}
}
