// remove_guard.go — `remove <plugin>` and the bundle ledger (P7-PLUG-20).
//
// Purpose: stop a single-plugin remove from leaving an installed bundle half
// installed, and keep the ledger in step after a plugin is removed.
// Inputs: the plugin directory and a plugin name.
// Outputs: ReferencingBundles (who needs the plugin), GuardPluginRemove (the
// E133 refusal or a warning), ForgetPlugin (drop the removed plugin's record).
// Constraints: reads go through Store.Load (a missing ledger is bootstrapped in
// memory and never written); ForgetPlugin writes only when the ledger file
// already exists and never creates it.
package bundle

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
)

// ReferencingBundles returns, sorted, the installed bundles whose install
// references the plugin (its installed_by), or nil when none does.
func ReferencingBundles(pluginDir, name string) ([]string, error) {
	l, _, err := newLedgerStore(pluginDir).Load()
	if err != nil {
		return nil, err
	}
	rec, ok := l.Plugins[strings.ToLower(strings.TrimSpace(name))]
	if !ok || len(rec.InstalledBy) == 0 {
		return nil, nil
	}
	return append([]string(nil), rec.InstalledBy...), nil
}

// GuardPluginRemove checks a plugin remove against the bundle ledger. In v1.5
// mode a plugin an installed bundle references is refused with E133 unless
// force is set; in v1.4 mode (or with force) a warning naming the bundles is
// written to w and the remove proceeds. A ledger that cannot be read cannot
// prove the plugin is unreferenced, so v1.5 without force returns that error.
func GuardPluginRemove(w io.Writer, pluginDir, name string, force bool) error {
	refs, err := ReferencingBundles(pluginDir, name)
	// compat.V15(P7-PLUG-20): remove <plugin> of a bundle member proceeds -> refuses with E133 unless --force
	strict := compat.V15() && !force
	if err != nil {
		if strict {
			return fmt.Errorf("cannot read the bundle ledger to check whether plugin %q is still referenced by a bundle: %w (pass --force to remove it anyway)", name, err)
		}
		_, _ = fmt.Fprintf(w, "warning: could not read the bundle ledger, so %q was not checked against installed bundles: %v\n", name, err)
		return nil
	}
	if len(refs) == 0 {
		return nil
	}
	list := strings.Join(refs, ", ")
	if strict {
		return errs.Newf("E133", "plugin %q is still referenced by bundle(s) %s", name, list).
			WithFix(fmt.Sprintf("run 'nself bundle remove %s' (plugins no other bundle needs go with it), or pass --force to remove only this plugin", refs[0]))
	}
	_, _ = fmt.Fprintf(w, "warning: plugin %q is still referenced by bundle(s) %s; removing it leaves the bundle(s) incomplete.\n", name, list)
	return nil
}

// errNoRecord aborts ForgetPlugin's Update when there is nothing to drop, so
// the ledger is not rewritten.
var errNoRecord = errors.New("no record")

// ForgetPlugin drops the plugin's record from the ledger after it was removed
// from disk. It does nothing when the ledger file does not exist (a plugin
// remove never bootstraps the ledger) or has no record of the plugin; a ledger
// file that cannot be stat'ed for another reason is an error.
func ForgetPlugin(pluginDir, name string) error {
	store := newLedgerStore(pluginDir)
	if _, err := os.Stat(store.Path()); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // no ledger yet: nothing to keep in step
		}
		return err // unreadable is not "no ledger"
	}
	key := strings.ToLower(strings.TrimSpace(name))
	err := store.Update(func(l *ledger.Ledger) error {
		if _, ok := l.Plugins[key]; !ok {
			return errNoRecord
		}
		delete(l.Plugins, key)
		return nil
	})
	if errors.Is(err, errNoRecord) {
		return nil
	}
	return err
}
