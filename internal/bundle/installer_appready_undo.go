package bundle

// installer_appready_undo.go — undo a bundle install whose reconcile failed.
//
// Purpose: a refused or failed reconcile (E403 without --yes, E480/E481, a
// build error) must not leave the members this install added on disk and in the
// ledger, or later builds would see them (D-0760, same class as D-0433).
// Inputs: the planned members, the plugins that were on disk before the install
// (name -> version) and the ledger before recording this install.
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
// agree. Prior ownership is restored for existing members; an incomplete undo
// keeps a newly recorded bundle so its remaining members can still be removed.
func undoFreshMembers(ctx context.Context, store *ledger.Store, pluginDir string, planned []string, prior ledger.Ledger, result *InstallResult, before map[string]string, out io.Writer) error {
	fresh := freshMembers(planned, before)
	var errsOut []error
	var gone []string
	for i := len(fresh) - 1; i >= 0; i-- {
		name := fresh[i]
		_, _ = fmt.Fprintf(out, "  ↩ rolling back %s (the build did not apply)...\n", name)
		if err := plugin.UndoInstall(ctx, nil, name, pluginDir); err != nil {
			errsOut = append(errsOut, fmt.Errorf("undoing the install of plugin %q: %w", name, err))
			continue
		}
		gone = append(gone, name)
	}
	result.RolledBack = append(result.RolledBack, gone...)
	if err := store.Update(func(l *ledger.Ledger) error {
		restoreInstallOwnership(l, prior, result.Bundle, planned, before, gone, len(gone) == len(fresh))
		return nil
	}); err != nil {
		errsOut = append(errsOut, fmt.Errorf("the plugins are removed but updating %s failed: %w", store.Path(), err))
	}
	result.Changed = !maps.Equal(before, buildInstalledVersionMap(pluginDir))
	return errors.Join(errsOut...)
}

// restoreInstallOwnership restores prior references while retaining facts for
// upgraded members and the records of fresh members whose undo failed.
func restoreInstallOwnership(l *ledger.Ledger, prior ledger.Ledger, slug string, planned []string, before map[string]string, gone []string, complete bool) {
	for _, name := range planned {
		key := strings.ToLower(name)
		if _, existed := before[key]; !existed {
			continue
		}
		rec := l.Plugins[key]
		old, recorded := prior.Plugins[key]
		rec.InstalledBy = old.InstalledBy
		rec.Explicit = old.Explicit || !recorded
		l.Plugins[key] = rec
	}
	for _, name := range gone {
		delete(l.Plugins, strings.ToLower(name))
	}
	if old, recorded := prior.Bundles[slug]; recorded {
		l.Bundles[slug] = old
	} else if complete {
		delete(l.Bundles, slug)
	}
}
