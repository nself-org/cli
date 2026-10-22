package reconcile

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Fixtures model local projects; CI-specific tests opt in explicitly.
	_ = os.Unsetenv("CI")
	_ = os.Unsetenv("NSELF_CI")
	os.Exit(m.Run())
}
