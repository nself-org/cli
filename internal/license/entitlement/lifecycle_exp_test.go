package entitlement

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// retainedToken rejects a non-positive exp before the grace check. The
// CONDITIONALS_BOUNDARY mutant at lifecycle.go:78:30 (Exp <= 0 to Exp < 0) only
// changes exp=0, and that grace deadline (1970-01-31) has passed, so it is
// equivalent; this pins the rejection itself and a live token as the control.
func TestRetainedTokenRejectsNonPositiveExp(t *testing.T) {
	sign := policySigner(t)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "accepted")
	path := filepath.Join(t.TempDir(), "entitlement")
	for _, tc := range []struct {
		name string
		exp  int64
		want bool
	}{
		{"zero", 0, false},
		{"negative", -1, false},
		{"live", time.Now().Add(time.Hour).Unix(), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(sign("accepted", []string{"chat"}, tc.exp)), 0600); err != nil {
				t.Fatal(err)
			}
			if got := retainedToken(path) != ""; got != tc.want {
				t.Fatalf("retained=%v, want %v", got, tc.want)
			}
		})
	}
}
