package bundle

// Purpose: P7-PLUG-20 fix: the remove guard, the ledger lookup and the ledger
// forget refuse path-shaped plugin names, so `./py` can never miss the ledger
// record of `py` while a removal still reaches the `py` directory (E133 bypass).
// Inputs: a temp plugin directory. Constraints: no network.

import (
	"io"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

func TestGuardPluginRemoveRefusesPathNames(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"./py", "../py", "py/", "/py", "", "Py", ".."} {
		for _, force := range []bool{false, true} {
			err := GuardPluginRemove(io.Discard, dir, name, force)
			if d := errs.Describe(err); d == nil || d.Code != "E401" {
				t.Errorf("GuardPluginRemove(%q, force=%v) = %v, want E401", name, force, err)
			}
		}
		if _, err := ReferencingBundles(dir, name); err == nil {
			t.Errorf("ReferencingBundles(%q) = nil error, want a refusal", name)
		}
		if err := ForgetPlugin(dir, name); err == nil {
			t.Errorf("ForgetPlugin(%q) = nil, want a refusal", name)
		}
	}
}
