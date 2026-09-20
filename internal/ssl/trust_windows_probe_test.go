package ssl

// trust_windows_probe_test.go — pins isCAInstalledWindows as a deliberate
// constant, not an accident.
//
// Purpose: the function matches the "failure becomes a plausible value" shape
//          being audited, but is a correct instance of it: the probe is
//          unimplemented and always answers "not installed". Document that by
//          test as well as by comment, so a later reader does not "fix" the
//          swallow by making it return an error — its callers have no error
//          path that would improve on a redundant idempotent install.
// Inputs:  any path, including one that does not exist.
// Outputs: always (false, nil).
// Constraints: runs on every OS; it never shells out.

import "testing"

func TestIsCAInstalledWindows_AlwaysReportsNotInstalled(t *testing.T) {
	// Both a real-looking path and an absent one must give the same answer:
	// the function inspects nothing, so there is no input that changes it.
	for _, p := range []string{"", "/tmp/nonexistent-rootCA-99999.pem", `C:\mkcert\rootCA.pem`} {
		installed, err := isCAInstalledWindows(p)
		if err != nil {
			t.Errorf("isCAInstalledWindows(%q) returned an error %v; it inspects nothing, "+
				"so it has no failure to report", p, err)
		}
		if installed {
			t.Errorf("isCAInstalledWindows(%q) = true; over-reporting 'installed' skips a needed "+
				"install and leaves the user with TLS errors. Under-reporting only costs one "+
				"redundant run of the idempotent installCAWindows.", p)
		}
	}
}
