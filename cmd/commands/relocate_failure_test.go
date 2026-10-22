package commands

import (
	"errors"
	"testing"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/plugin"
)

func TestRelocateExecFailureKeepsCode(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{{"human", nil}, {"json", []string{"--json"}}} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _ = relocateSetup(t, "", nil)
			t.Setenv("HOME", t.TempDir())
			autoAddPlugin = func(string, string) (string, error) {
				return plugin.PublishedBinaryPath("nself-ci"), nil
			}
			handled, err := relocateBreakout("ci", tc.args)
			var coded *errs.CLIError
			var silent errs.Silencer
			if !handled || !errors.As(err, &coded) || coded.Code != "E406" || (errors.As(err, &silent) && silent.Silent()) {
				t.Fatalf("handled=%v err=%v: want reportable E406", handled, err)
			}
		})
	}
}

func TestWarningsStopAtTerminator(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{[]string{"nself", "ci", "--quiet"}, true},
		{[]string{"nself", "ci", "--", "--quiet"}, false},
		{[]string{"nself", "ci", "--no-deprecation-warnings"}, true},
		{[]string{"nself", "ci", "--", "--no-deprecation-warnings"}, false},
	} {
		if got := warningsSilenced(tc.args); got != tc.want {
			t.Errorf("%v: got %v, want %v", tc.args, got, tc.want)
		}
	}
}
