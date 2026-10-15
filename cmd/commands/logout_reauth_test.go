package commands

// Purpose: D-0761 regression coverage for logout warnings and credential removal.
// Inputs: both logout routes, --all modes, and rejected/server-error responses.
// Outputs: successful local logout with a context-appropriate server warning.
// Constraints: fake credentials and an isolated HOME; no parallel global output capture.

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/auth"
	"github.com/spf13/cobra"
)

func TestLogout_ServerFailureStillRemovesCredentials(t *testing.T) {
	for _, route := range []struct {
		name string
		run  func(*cobra.Command, []string) error
	}{{"logout", runLogout}, {"account logout", runAccountLogout}} {
		for _, all := range []bool{false, true} {
			for _, status := range []int{http.StatusUnauthorized, http.StatusInternalServerError} {
				t.Run(fmt.Sprintf("%s/all=%t/status=%d", route.name, all, status), func(t *testing.T) {
					home := isolateHome(t)
					writeTestAuthFile(t, home, &auth.AuthFile{AccessToken: "fake-token"})
					setAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodPost || r.URL.Path != "/auth/signout" {
							t.Errorf("unexpected request: %s %s", r.Method, r.URL)
						}
						if got := r.URL.Query().Get("all") == "true"; got != all {
							t.Errorf("revoke all = %t, want %t", got, all)
						}
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(status)
						_, _ = w.Write([]byte(`{"error":"server_failure","message":"server unavailable"}`))
					})
					cmd := &cobra.Command{}
					cmd.Flags().Bool("all", all, "")
					var runErr error
					warnings := captureStderr(t, func() {
						_, runErr = captureStdout(t, func() error { return route.run(cmd, nil) })
					})
					if runErr != nil {
						t.Fatalf("logout: %v", runErr)
					}
					if _, err := os.Stat(filepath.Join(home, ".nself", "auth.json")); !os.IsNotExist(err) {
						t.Errorf("credentials remain after logout: %v", err)
					}
					want := "Could not revoke session on server:"
					if status == http.StatusUnauthorized {
						want = "session already invalid on the server"
					}
					if !strings.Contains(warnings, want) || !strings.Contains(warnings, "Deleting local credentials anyway.") {
						t.Errorf("warnings %q must contain %q and confirm credential deletion", warnings, want)
					}
					if strings.Contains(warnings, "nself login") {
						t.Errorf("logout must not recommend logging in: %s", warnings)
					}
				})
			}
		}
	}
}
