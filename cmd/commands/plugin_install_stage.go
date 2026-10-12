package commands

// Purpose: undo what one `nself plugin install` / `nself add` run put on disk
// when the reconcile that follows it is refused (D-0433), and hold
// finishPluginInstall (moved here to keep plugin_install.go under the line cap).
// Inputs: the plugin directory, the config, and the reconcile function.
// Outputs: the reconcile error (always returned); one stderr line naming the
// plugins that were removed again.
// Constraints: a literal temp-dir stage is not possible because the reconcile
// plans from the plugin dir, so the plugin has to be on disk for the plan. The
// stage instead records which plugin directories exist before the install and
// removes only the ones that appeared during it (dependencies included).
// Plugins that were already installed are never touched, so an in-place
// upgrade of one is not undone. Any reconcile error undoes, not only the
// refusals (E403/E480/E481): the plugin is kept only once the plan is confirmed
// and the write succeeded. The snapshot is not taken under the install lock, so
// a second process installing into the same plugin dir during a refused run
// would have its new plugin undone too. v1.4 never reconciles here, so it
// never undoes anything.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/plugin"
)

// undoPluginInstall reverses one fresh install; a seam so tests can fail it.
var undoPluginInstall = plugin.UndoInstall

// installStage remembers which plugin directories existed before an install run.
type installStage struct {
	pluginDir string
	before    map[string]bool
}

// beginInstallStage snapshots the plugin subdirectories of pluginDir. A missing
// directory is an empty set.
func beginInstallStage(pluginDir string) *installStage {
	return &installStage{pluginDir: pluginDir, before: stagePluginNames(pluginDir)}
}

// stagePluginNames lists the plugin subdirectories of dir as a set. Dot
// directories and ".prev" backups are not plugins.
func stagePluginNames(dir string) map[string]bool {
	names := map[string]bool{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return names
	}
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() || strings.HasPrefix(n, ".") || strings.HasSuffix(n, ".prev") {
			continue
		}
		names[n] = true
	}
	return names
}

// created returns, sorted, the plugin directories present now that were not
// present when the stage began.
func (s *installStage) created() []string {
	var out []string
	for n := range stagePluginNames(s.pluginDir) {
		if !s.before[n] {
			out = append(out, n)
		}
	}
	sort.Strings(out)
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
	names := s.created()
	if len(names) == 0 {
		return rerr
	}
	errsOut := []error{rerr}
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
