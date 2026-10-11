package report

// main_test.go: isolates HOME for the whole package so no test can read the
// real ~/.nself/license/key (the stored licence key is a scrub input).
// TestMain lives in child_unix_test.go (it also serves as the slow doctor
// child) and in main_other_test.go (other platforms); both call isolateHome.

import (
	"fmt"
	"os"
)

// isolateHome points HOME at a fresh temp dir and clears the licence env vars;
// the returned func removes the dir.
func isolateHome() (cleanup func()) {
	home, err := os.MkdirTemp("", "report-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = os.Setenv("HOME", home)
	_ = os.Setenv("USERPROFILE", home)
	for _, k := range []string{"NSELF_PLUGIN_LICENSE_KEY", "NSELF_LICENSE_KEY_1", "NSELF_LICENSE_KEY_2"} {
		_ = os.Unsetenv(k)
	}
	return func() { _ = os.RemoveAll(home) }
}
