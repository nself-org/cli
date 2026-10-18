package commands

// Purpose: TestBreakoutCIEval (P7-CI-45). In v1.5 `nself ci eval ...` leaves the
// ci plugin and runs the eval-gate plugin's `nself eval ...`, through the
// breakouts row `ci eval -> nself-eval-gate eval`. Same fixtures as
// relocate_test.go: a stub binary stands in for the plugin, no network.

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/plugin"
)

// evalStubSeam publishes stubScript as the eval-gate binary (nself-eval) and
// records the install request.
func evalStubSeam(t *testing.T) func(string, string) (string, error) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	return func(string, string) (string, error) {
		p := plugin.PublishedBinaryPath("nself-eval")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return "", err
		}
		return p, os.WriteFile(p, []byte(stubScript), 0o755)
	}
}

func TestBreakoutCIEval(t *testing.T) {
	t.Run("row", func(t *testing.T) {
		for _, tc := range []struct {
			argv       []string
			plugin     string
			pluginArgs []string
		}{
			{[]string{"ci", "eval", "--help"}, "nself-eval-gate", []string{"--help"}},
			{[]string{"ci", "eval", "gate", "--tier", "supervised"}, "nself-eval-gate", []string{"gate", "--tier", "supervised"}},
			{[]string{"ci", "eval", "--json"}, "nself-eval-gate", []string{"--json"}},
			{[]string{"eval", "gate", "--tier", "full-auto"}, "nself-eval-gate", []string{"gate", "--tier", "full-auto"}},
			{[]string{"ci", "build", "--upload"}, "ci", []string{"build", "--upload"}},
			{[]string{"ci", "--check"}, "ci", []string{"--check"}},
		} {
			row, got, ok := breakoutRow(tc.argv[0], tc.argv[1:])
			if !ok || row.Plugin != tc.plugin || !reflect.DeepEqual(got, tc.pluginArgs) {
				t.Errorf("%v: plugin %q args %v ok=%v, want %q %v", tc.argv, row.Plugin, got, ok, tc.plugin, tc.pluginArgs)
			}
		}
	})
	t.Run("installed plugin runs nself-eval", func(t *testing.T) {
		proof, calls, warn := relocateSetup(t, "", nil)
		autoAddPlugin = recordOver(calls, evalStubSeam(t))
		handled, err := relocateInvocation([]string{"ci", "eval", "--help"})
		if !handled || err != nil {
			t.Fatalf("handled=%v err=%v", handled, err)
		}
		if !reflect.DeepEqual(*calls, []seamCall{{"nself-eval-gate", ""}}) {
			t.Fatalf("seam calls %v", *calls)
		}
		if got := readProof(t, proof)["argv"]; got != "--help" {
			t.Fatalf("nself-eval argv %q, want --help", got)
		}
		if w := warn.String(); !strings.Contains(w, "'nself ci eval' now comes from the nself-eval-gate plugin") {
			t.Fatalf("deprecation line %q", w)
		}
	})
	t.Run("not installed and auto-add off is E404", func(t *testing.T) {
		_, calls, _ := relocateSetup(t, "", nil)
		t.Setenv(envNoAutoAdd, "1")
		handled, err := relocateInvocation([]string{"ci", "eval", "--help"})
		if !handled {
			t.Fatal("ci eval must be handled as a break-out")
		}
		assertBreakoutRefused(t, err, "nself-eval-gate")
		if code := aiCode(err); code != "E404" || len(*calls) != 0 {
			t.Fatalf("code %q calls %v", code, *calls)
		}
		if !strings.Contains(err.Error(), "nself ci eval") {
			t.Errorf("error does not name the typed command: %v", err)
		}
	})
}
