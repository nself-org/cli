package compattest_test

import (
	"os"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/cli/sdk/go/v2/compat/compattest"
)

// TestBothRunsOncePerMode checks fn runs once per mode and sees the matching
// V15() and Mode() values.
func TestBothRunsOncePerMode(t *testing.T) {
	seen := map[string]int{}
	compattest.Both(t, func(t *testing.T) {
		name := t.Name()
		want := name[len(name)-4:]
		if got := compat.Mode(); got != want {
			t.Fatalf("subtest %s saw mode %s", name, got)
		}
		if compat.V15() != (want == "v1.5") {
			t.Fatalf("subtest %s saw V15()=%v", name, compat.V15())
		}
		seen[want]++
	})
	if seen["v1.4"] != 1 || seen["v1.5"] != 1 {
		t.Fatalf("want one run per mode, got %v", seen)
	}
}

// TestSetRestoresValue checks Set puts the previous value back after the test.
func TestSetRestoresValue(t *testing.T) {
	t.Setenv(compat.EnvVar, "keep-me")
	t.Run("inner", func(t *testing.T) {
		compattest.Set(t, true)
		if !compat.V15() {
			t.Fatal("Set(true) did not select v1.5")
		}
	})
	if got := os.Getenv(compat.EnvVar); got != "keep-me" {
		t.Fatalf("value not restored, got %q", got)
	}
}
