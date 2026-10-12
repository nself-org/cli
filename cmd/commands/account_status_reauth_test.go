package commands

// Purpose: table tests for D-0480. A 401 from the auth server (rejected device
// token: revoked, or the signing key rotated) must reach every account command
// as the one re-login message naming `nself login --force`; a 403 stays the
// tier denial. Fake token only; the auth server is an httptest stub.

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/auth"
	"github.com/spf13/cobra"
)

// withStdin replaces os.Stdin with input for the duration of the test.
func withStdin(t *testing.T, input string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(input); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old; _ = r.Close() })
}

type reauthCase struct {
	name  string
	cmd   *cobra.Command
	flags map[string]string
	args  []string
	// tier is true when a 403 must surface as the tier denial (commands that
	// go through handleAuthError). `account status` falls back to cached info.
	tier bool
}

func reauthCases() []reauthCase {
	return []reauthCase{
		{name: "status", cmd: accountStatusCmd},
		{name: "devices list", cmd: accountDevicesCmd, tier: true},
		{name: "devices revoke", cmd: accountDevicesCmd, flags: map[string]string{"revoke": "dev-1"}, tier: true},
		{name: "licenses list", cmd: accountLicensesCmd, tier: true},
		{name: "licenses activate", cmd: accountLicensesCmd, flags: map[string]string{"activate": "lic-1"}, tier: true},
		{name: "team list", cmd: accountTeamCmd, tier: true},
		{name: "team invite", cmd: accountTeamCmd, flags: map[string]string{"invite": "bob@example.com"}, tier: true},
		{name: "transfer", cmd: accountTransferCmd, args: []string{"key-1", "bob@example.com"}, tier: true},
	}
}

func TestAccountCommands_RejectedTokenSaysLoginForce(t *testing.T) {
	for _, tc := range reauthCases() {
		t.Run(tc.name, func(t *testing.T) {
			home := isolateHome(t)
			writeTestAuthFile(t, home, &auth.AuthFile{AccessToken: "fake-token-not-real", Email: "alice@example.com", Tier: "pro"})
			setAuthServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"invalid_token","message":"token rejected"}`))
			})
			withStdin(t, "y\n")
			for k, v := range tc.flags {
				if err := tc.cmd.Flags().Set(k, v); err != nil {
					t.Fatal(err)
				}
				k := k
				t.Cleanup(func() { _ = tc.cmd.Flags().Set(k, "") })
			}

			err := tc.cmd.RunE(tc.cmd, tc.args)
			if err == nil {
				t.Fatal("expected an error for a rejected token, got nil")
			}
			if !strings.Contains(err.Error(), "nself login --force") {
				t.Errorf("error %q does not name `nself login --force`", err)
			}
			for _, bad := range []string{"tier", "reach", "permission denied"} {
				if strings.Contains(err.Error(), bad) {
					t.Errorf("error %q still contains misleading %q", err, bad)
				}
			}
		})
	}
}

// TestAccountCommands_ForbiddenStaysTierDenial is the positive control: the
// same stub with 403 must NOT produce the re-login message.
func TestAccountCommands_ForbiddenStaysTierDenial(t *testing.T) {
	for _, tc := range reauthCases() {
		if !tc.tier {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			home := isolateHome(t)
			writeTestAuthFile(t, home, &auth.AuthFile{AccessToken: "fake-token-not-real", Email: "alice@example.com", Tier: "free"})
			setAuthServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"tier","message":"upgrade"}`))
			})
			withStdin(t, "y\n")
			for k, v := range tc.flags {
				if err := tc.cmd.Flags().Set(k, v); err != nil {
					t.Fatal(err)
				}
				k := k
				t.Cleanup(func() { _ = tc.cmd.Flags().Set(k, "") })
			}

			err := tc.cmd.RunE(tc.cmd, tc.args)
			if err == nil || !strings.Contains(err.Error(), "your tier does not include this feature") {
				t.Fatalf("403 should be the tier denial, got %v", err)
			}
			if strings.Contains(err.Error(), "login --force") {
				t.Errorf("403 must not tell the user to log in again: %v", err)
			}
		})
	}
}

// TestHandleAuthError_Mapping pins the status mapping without any HTTP.
func TestHandleAuthError_Mapping(t *testing.T) {
	cases := []struct {
		name string
		in   error
		want string
	}{
		{"not logged in (401 mapped by the client)", auth.ErrNotLoggedIn, "nself login --force"},
		{"403 tier", &auth.AuthAPIError{Status: 403}, "tier does not include"},
		{"429", &auth.AuthAPIError{Status: 429}, "rate limited"},
		{"500", &auth.AuthAPIError{Status: 500}, "try again later"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := handleAuthError(tc.in)
			if got == nil || !strings.Contains(got.Error(), tc.want) {
				t.Errorf("handleAuthError(%v) = %v, want substring %q", tc.in, got, tc.want)
			}
		})
	}
}
