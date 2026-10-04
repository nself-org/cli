package compattest_test

import (
	"os"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
)

// TestBothRunsOncePerMode checks fn runs exactly once per mode and observes the
// matching compat.V15() value.
func TestBothRunsOncePerMode(t *testing.T) {
	seen := map[string]int{}
	compattest.Both(t, func(t *testing.T) {
		want := t.Name()[len(t.Name())-4:]
		if got := compat.Mode(); got != want {
			t.Fatalf("subtest %s observed mode %s", t.Name(), got)
		}
		if (want == "v1.5") != compat.V15() {
			t.Fatalf("subtest %s: V15()=%v", t.Name(), compat.V15())
		}
		seen[want]++
	})
	if seen["v1.4"] != 1 || seen["v1.5"] != 1 || len(seen) != 2 {
		t.Fatalf("runs per mode = %v", seen)
	}
}

// TestSetIsRestored checks Set restores the previous value after the test.
func TestSetIsRestored(t *testing.T) {
	t.Setenv(compat.EnvVar, "0")
	t.Run("inner", func(t *testing.T) {
		compattest.Set(t, true)
		if !compat.V15() {
			t.Fatal("Set(true) did not enable v1.5")
		}
	})
	if v := os.Getenv(compat.EnvVar); v != "0" {
		t.Fatalf("after subtest NSELF_V15 = %q, want 0", v)
	}
	if compat.V15() {
		t.Fatal("V15 still true after inner test")
	}
}
