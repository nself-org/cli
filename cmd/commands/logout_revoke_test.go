package commands

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/auth"
	"github.com/nself-org/cli/internal/health"
	"github.com/spf13/cobra"
)

func TestLogoutRevokeWarning(t *testing.T) {
	for _, err := range []error{auth.ErrNotLoggedIn, fmt.Errorf("revoke: %w", auth.ErrNotLoggedIn)} {
		got := auth.LogoutRevokeWarning(err)
		if !strings.Contains(got, "session already invalid on the server") || strings.Contains(got, "nself login") {
			t.Fatalf("rejected session warning = %q", got)
		}
	}
	if got := auth.LogoutRevokeWarning(errors.New("service unavailable")); !strings.Contains(got, "service unavailable") {
		t.Fatalf("other revoke error lost: %q", got)
	}
}

func TestLogoutRejectedSessionDeletesCredentials(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(*cobra.Command, []string) error
	}{
		{"logout", runLogout},
		{"account logout", runAccountLogout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setHome(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/auth/signout" {
					t.Errorf("unexpected revoke path: %s", r.URL.Path)
				}
				w.WriteHeader(http.StatusUnauthorized)
			}))
			defer srv.Close()
			t.Setenv("NSELF_AUTH_SERVER_URL", srv.URL)
			if err := auth.WriteAuthFile(&auth.AuthFile{AccessToken: "fixture-token"}); err != nil {
				t.Fatal(err)
			}
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			old := os.Stderr
			os.Stderr = w
			t.Cleanup(func() { os.Stderr = old })
			cmd := &cobra.Command{}
			cmd.Flags().Bool("all", false, "")
			_, runErr := captureStdout(t, func() error { return tc.run(cmd, nil) })
			_ = w.Close()
			os.Stderr = old
			warning, readErr := io.ReadAll(r)
			if runErr != nil || readErr != nil {
				t.Fatalf("logout: %v, capture: %v", runErr, readErr)
			}
			if !strings.Contains(string(warning), "session already invalid on the server") || strings.Contains(string(warning), "nself login") {
				t.Fatalf("logout warning = %q", warning)
			}
			if _, err := auth.ReadAuthFile(); !errors.Is(err, auth.ErrNotLoggedIn) {
				t.Fatalf("credentials remain after rejected revoke: %v", err)
			}
		})
	}
}

func TestStatusExitV14Starting(t *testing.T) {
	t.Setenv("NSELF_V15", "0")
	results := []health.HealthResult{{Service: "hasura", Status: "starting"}}
	if got := statusExit(results, true, false); got != 2 {
		t.Fatalf("v1.4 starting status = %d, want 2", got)
	}
}
