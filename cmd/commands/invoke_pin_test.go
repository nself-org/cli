package commands

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/spf13/cobra"
)

// pinRoot is the mounted fixture tree with the real root's persistent --json
// bool, as RootCmd has it: cobra's lookup then knows --json takes no value.
func pinRoot(t *testing.T) *cobra.Command {
	t.Helper()
	r, _ := mountRoot(t, mountFixture(t))
	r.PersistentFlags().Bool("json", false, "machine output")
	return r
}

// TestInvokePin covers the child side of the path pin: no variable is no pin,
// a matching resolution runs, a shifted or unknown one is E421, and the
// variable never survives the check (grandchildren must not inherit it).
func TestInvokePin(t *testing.T) {
	r := pinRoot(t)
	cases := []struct {
		name, pin string
		set       bool
		argv      []string
		ok        bool
	}{
		{"no pin", "", false, []string{"demo", "--json", "--verbose", "--", "sub"}, true},
		{"match", "demo", true, []string{"demo", "--json", "--verbose=true", "--", "sub"}, true},
		{"match leaf", "demo sub", true, []string{"demo", "sub", "--json", "--yes=true"}, true},
		{"bare flag shifts the lookup", "demo", true, []string{"demo", "--json", "--verbose", "--", "sub"}, false},
		{"unknown command", "gone", true, []string{"gone", "--json"}, false},
		{"stale leaf grew a child", "demo nested", true, []string{"demo", "nested", "--json", "two"}, false},
	}
	for _, c := range cases {
		if c.set {
			t.Setenv(invoke.PathEnv, c.pin)
		} else {
			os.Unsetenv(invoke.PathEnv)
		}
		err := checkInvokePin(r, c.argv)
		if (err == nil) != c.ok {
			t.Errorf("%s: err=%v, want ok=%v", c.name, err, c.ok)
		}
		if err != nil {
			var e *errs.CLIError
			if !errors.As(err, &e) || e.Code != "E421" || !strings.Contains(err.Error()+e.Why, "resolved") {
				t.Errorf("%s: want E421 naming the resolved path, got %v", c.name, err)
			}
		}
		if _, still := os.LookupEnv(invoke.PathEnv); still {
			t.Errorf("%s: %s must be removed from the environment", c.name, invoke.PathEnv)
		}
	}
}
