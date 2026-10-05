package license

import (
	"os"
	"testing"

	"github.com/nself-org/cli/internal/compat"
)

// TestMain pins the package's legacy tests to v1.4 decisions. They were
// written against unsigned caches and unsigned stub servers, which v1.5
// refuses by design; tests of v1.5 behaviour select it themselves with
// compattest.Set or compattest.Both, which override this per test.
func TestMain(m *testing.M) {
	_ = os.Unsetenv(compat.EnvVar)
	os.Exit(m.Run())
}
