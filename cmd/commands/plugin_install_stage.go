package commands

// Purpose: undo what one `nself plugin install` / `nself add` run put on disk
// when the reconcile that follows it is refused (D-0433), and hold
// finishPluginInstall (moved here to keep plugin_install.go under the line cap).
// Inputs: the plugin directory, the config, and the reconcile function.
// Outputs: the reconcile error (always returned); one stderr line naming the
// plugins that were removed again.
// Constraints: a literal temp-dir stage is not possible because the reconcile
// plans from the plugin dir, so the plugin has to be on disk for the plan. The
// stage records newly created directories during each locked install call
// (dependencies included), then rolls back only those directory identities.
// Plugins that were already installed are never touched, so an in-place
// upgrade of one is not undone. Any reconcile error undoes, not only the
// refusals (E403/E480/E481): the plugin is kept only once the plan is confirmed
// and the write succeeded. Command install/update and bundle operations share
// the operation lock, so another invocation cannot enter an ownership capture.
// v1.4 never reconciles here, so it never undoes anything.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/plugin"
)

// undoPluginInstall reverses one fresh install; a seam so tests can fail it.
var undoPluginInstall = plugin.UndoInstall

// installStage remembers only directory identities created by this invocation.
type installStage struct {
	pluginDir string
	owned     map[string]os.FileInfo
}

// beginInstallStage starts an empty ownership record, without a racy snapshot.
func beginInstallStage(pluginDir string) *installStage {
	return &installStage{pluginDir: pluginDir, owned: map[string]os.FileInfo{}}
}

// install captures newly created directories under the shared operation lock.
// Capture also runs on failure: a failed target may leave installed dependencies.
func (s *installStage) install(ctx context.Context, fn func() error) error {
	return withPluginInstallOperation(ctx, s.pluginDir, func() error {
		before := stagePluginNames(s.pluginDir)
		err := fn()
		for name := range stagePluginNames(s.pluginDir) {
			previous, owned := before[name], s.owned[name]
			if previous == nil || (owned != nil && os.SameFile(owned, previous)) {
				if info, statErr := os.Stat(filepath.Join(s.pluginDir, name)); statErr == nil {
					s.owned[name] = info
				}
			}
		}
		return err
	})
}

// stagePluginNames lists plugin directory identities. Dot directories, shared
// command binaries, and ".prev" backups are not plugin installations.
func stagePluginNames(dir string) map[string]os.FileInfo {
	names := map[string]os.FileInfo{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return names
	}
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() || n == "bin" || strings.HasPrefix(n, ".") || strings.HasSuffix(n, ".prev") {
			continue
		}
		if info, err := e.Info(); err == nil {
			names[n] = info
		}
	}
	return names
}

// created returns owned directories still present with their original identity.
// A replacement installed by a later invocation must survive this rollback.
func (s *installStage) created() []string {
	var out []string
	for n, owned := range s.owned {
		if now, err := os.Stat(filepath.Join(s.pluginDir, n)); err == nil && os.SameFile(owned, now) {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// readinessNames includes fresh dependencies, keeping explicit names first.
// The ledger still records only the explicit installs supplied by the caller.
func (s *installStage) readinessNames(installed []string) []string {
	out := append([]string(nil), installed...)
	seen := make(map[string]bool, len(out))
	for _, name := range out {
		seen[name] = true
	}
	for _, name := range s.created() {
		if name != "bin" && !seen[name] {
			out = append(out, name)
			seen[name] = true
		}
	}
	return out
}

// reconcile runs reconcileFn. When it fails, every plugin this run created is
// removed again (files and command binary; never the schema) and dropped from
// the bundle ledger, so a refused build does not leave a half-added plugin
// behind. The reconcile error is always returned; undo failures are joined.
func (s *installStage) reconcile(ctx context.Context, cfg *config.Config, w io.Writer, reconcileFn func() error) error {
	rerr := reconcileFn()
	if rerr == nil {
		return nil
	}
	return errors.Join(rerr, withPluginInstallOperation(ctx, s.pluginDir, func() error {
		return s.undo(ctx, cfg, w)
	}))
}

// undo checks ownership and removes directories while other operations wait.
func (s *installStage) undo(ctx context.Context, cfg *config.Config, w io.Writer) error {
	names := s.created()
	if len(names) == 0 {
		return nil
	}
	var errsOut []error
	for _, name := range names {
		if err := undoPluginInstall(ctx, cfg, name, s.pluginDir); err != nil {
			errsOut = append(errsOut, fmt.Errorf("undoing the install of plugin %q: %w", name, err))
			continue
		}
		if err := bundle.ForgetPlugin(s.pluginDir, name); err != nil {
			errsOut = append(errsOut, fmt.Errorf("plugin %q is removed but the bundle ledger could not be updated: %w", name, err))
		}
	}
	_, _ = fmt.Fprintf(w, "nself: the build did not apply, so these plugins were removed again: %s\n", strings.Join(names, ", "))
	return errors.Join(errsOut...)
}

// finishPluginInstall reconciles once after any successful install (v1.5) and
// reports every failure together: a failed reconcile never hides the plugins
// that failed to install, and vice versa (errors.Join, as bundle install does).
func finishPluginInstall(v15 bool, installed, failures []string, reconcileFn func() error) error {
	var all []error
	if v15 && len(installed) > 0 {
		if err := reconcileFn(); err != nil {
			all = append(all, err)
		}
	}
	if len(failures) > 0 {
		all = append(all, fmt.Errorf("failed to install: %s", strings.Join(failures, ", ")))
	}
	return errors.Join(all...)
}
