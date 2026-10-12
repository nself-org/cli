package stack

// Purpose: after readiness, track the manifest tables of the active boot-migration
// plugins in Hasura, as the PLUG epic requires of `update stack` (P7-PLUG-31).
// Inputs: installed plugin manifests and the running Compose services.
// Outputs: an error naming the plugin and table Hasura rejected.
// Constraints: tracking only, nothing is untracked; the concurrent-metadata retry
// (D-0389) lives in database.TrackPluginTables and is reused, not copied.

import (
	"context"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/bundle/appready"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/database"
)

// runtimeTrackTables is the shared tracker, with the one export_metadata parser.
var runtimeTrackTables = func(ctx context.Context, cfg *config.Config, plugins []database.PluginTables) (database.TrackResult, error) {
	return database.TrackPluginTables(ctx, cfg, plugins, appready.TrackedTables)
}

// trackTables tracks the tables of active plugins that apply migrations at boot.
// Inputs: running services and plugin manifests. Outputs: nil when nothing declares tables.
// Constraints: a plugin without migrations.apply: boot is skipped, as `plugin install` does.
func (r *Runtime) trackTables(ctx context.Context) error {
	dir, entries, err := r.pluginEntries()
	if err != nil {
		return err
	}
	if entries == nil {
		return nil
	}
	c, err := r.compose()
	if err != nil {
		return err
	}
	specs, err := runtimeServiceSpecs(ctx, c, r.dir)
	if err != nil {
		return fmt.Errorf("resolve active plugin services for table tracking: %w", err)
	}
	active := map[string]bool{}
	for _, spec := range specs {
		active[spec.Name] = true
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && (active[e.Name()] || active["plugin_"+e.Name()]) {
			names = append(names, e.Name())
		}
	}
	members, err := appready.LoadMembers(dir, names)
	if err != nil {
		return fmt.Errorf("read plugin manifests for table tracking: %w", err)
	}
	var plugins []database.PluginTables
	for _, m := range members {
		if m.BootMigrations && len(m.Tables) > 0 {
			plugins = append(plugins, database.PluginTables{Plugin: m.Name, Tables: m.Tables})
		}
	}
	if len(plugins) == 0 {
		return nil
	}
	if _, err := runtimeTrackTables(ctx, r.cfg, plugins); err != nil {
		return fmt.Errorf("track plugin tables in Hasura (%s): %w", pluginNames(plugins), err)
	}
	return nil
}

// pluginNames lists plugin slugs for an error message.
func pluginNames(plugins []database.PluginTables) string {
	names := make([]string, 0, len(plugins))
	for _, p := range plugins {
		names = append(names, p.Plugin)
	}
	return strings.Join(names, ", ")
}
