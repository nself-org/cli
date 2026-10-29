package commands

// Purpose: `license refresh` against an unreachable ping_api prints only the
// fixed failure code, never the key, its mask, its hash or the request URL.
// Inputs: the grace fixture with its server shut down. Outputs: pass/fail.
// Constraints: loopback only.

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/license"
)

func TestLicenseRefreshTransportFailureIsFixedCode(t *testing.T) {
	f := newGraceFixture(t)
	t.Setenv("NSELF_PING_API_URL", "http://127.0.0.1:1") // nothing listens
	stdout, stderr, err := runRefresh(t, f.project, map[string]string{"project": f.project, "quiet": "true"})
	if stdout != "" || stderr != "entitlement refresh failed: ping-unavailable\n" || errs.ExitCodeFor(err) != 1 {
		t.Fatalf("stdout %q stderr %q exit %d", stdout, stderr, errs.ExitCodeFor(err))
	}
	for _, s := range []string{graceKey, license.MaskKey(graceKey), license.HashKey(graceKey), "127.0.0.1"} {
		if strings.Contains(stdout+stderr, s) {
			t.Errorf("output contains %q", s)
		}
	}
}
