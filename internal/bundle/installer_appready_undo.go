package bundle

// installer_appready_undo.go — undo a bundle install whose reconcile failed.
//
// Purpose: a refused or failed reconcile (E403 without --yes, E480/E481, a
// build error) must not leave the members this install added on disk and in the
// ledger, or later builds would see them (D-0760, same class as D-0433).
// Inputs: the planned members, the plugins that were on disk before the install
// (name -> version) and whether the ledger already recorded the bundle.
// Outputs: the fresh members removed again (files and command binary) and their
// ledger records dropped; the reconcile error stays the caller's error.
// Constraints: only members absent before the install are undone; a plugin that
// was already on disk (same version, kept higher version or an in-place upgrade)
// stays installed. The undo never drops a schema: an install reuses a schema
// kept by an earlier remove, so plugin.UndoInstall leaves it alone.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"strings"

	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/plugin"
)

// freshMembers returns the planned members that were not on disk before the install.
func freshMembers(planned []string, before map[string]string) []string {
	var fresh []string
	for _, name := range planned {
		if _, there := before[strings.ToLower(name)]; !there {
			fresh = append(fresh, name)
		}
	}
	return fresh
}

// undoFreshMembers removes the fresh members from disk (in reverse install
// order) and from the ledger, and returns the first-line errors joined. A
// member that could not be undone keeps its ledger record so disk and ledger
// agree. wasRecorded says whether the ledger held the bundle before this
// install; only then does the bundle record stay.
func undoFreshMembers(ctx context.Context, cfg *config.Config, opts InstallOpts, store *ledger.Store, pluginDir string, fresh []string, wasRecorded bool, bundleSlug string, result *InstallResult, before map[string]string, out io.Writer) error {
	undo := opts.undoer
	if undo == nil {
		undo = plugin.UndoInstall
	}
	var errsOut []error
	var gone []string
	for i := len(fresh) - 1; i >= 0; i-- {
		name := fresh[i]
		_, _ = fmt.Fprintf(out, "  ↩ rolling back %s (the build did not apply)...\n", name)
		if err := undo(ctx, cfg, name, pluginDir); err != nil {
			errsOut = append(errsOut, fmt.Errorf("undoing the install of plugin %q: %w", name, err))
			continue
		}
		gone = append(gone, name)
	}
	result.RolledBack = append(result.RolledBack, gone...)
	if len(gone) > 0 {
		if err := store.Update(func(l *ledger.Ledger) error {
			for _, name := range gone {
				delete(l.Plugins, strings.ToLower(name))
			}
			if !wasRecorded {
				// Unknown bundle (nothing was recorded) is fine; the plugins are gone already.
				_, _ = l.RemoveBundle(bundleSlug)
			}
			return nil
		}); err != nil {
			errsOut = append(errsOut, fmt.Errorf("the plugins are removed but updating %s failed: %w", store.Path(), err))
		}
	}
	result.Changed = !maps.Equal(before, buildInstalledVersionMap(pluginDir))
	return errors.Join(errsOut...)
}
