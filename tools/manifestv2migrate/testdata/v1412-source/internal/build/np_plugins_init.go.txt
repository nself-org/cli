// np_plugins_init.go — np_plugins seed SQL generator.
//
// Writes an idempotent SQL seed script that ensures one row exists in
// np_plugins for every plugin installed under pluginDir. The script is named
// 05-np-plugins-seed.sql so it runs strictly AFTER 04-np-plugins.sql (which
// CREATE TABLE IF NOT EXISTS np_plugins ... ) emitted by
// internal/postgres/generator.go.
//
// Idempotency is guaranteed by INSERT ... ON CONFLICT (name) DO NOTHING.
// Re-running `nself build` produces the same script with the same content;
// re-running `nself start` (or, equivalently, re-applying init scripts on a
// rebuilt postgres volume) inserts no duplicate rows.
//
// Why a "seed empty row" matters: downstream consumers (admin UI, doctor,
// plugin loader) join np_plugins by name. When a plugin is installed on disk
// but no row exists yet, those joins drop the plugin from views. Seeding an
// empty row at build time keeps every installed plugin discoverable from SQL,
// even before its first runtime touch.
package build

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// npPluginsSeedFilename is the file name produced by GenerateNpPluginsSeed.
// The "05-" prefix orders it after 04-np-plugins.sql (CREATE TABLE) and any
// future tables added under 05-09 init slots.
const npPluginsSeedFilename = "05-np-plugins-seed.sql"

// GenerateNpPluginsSeed scans pluginDir for installed plugins (each is a
// subdir with a plugin.json manifest), determines a tier ('free' / 'pro') for
// each, and writes postgres/init/05-np-plugins-seed.sql. The script uses
// INSERT ... ON CONFLICT (name) DO NOTHING so re-runs are no-ops.
//
// If pluginDir is missing or empty, the function still writes a header-only
// file (a single comment + no INSERTs). That keeps the path stable across
// builds — operators inspecting postgres/init/ won't see the file vanish
// when they uninstall every plugin.
//
// Returns the absolute path of the file that was written.
func GenerateNpPluginsSeed(workdir, pluginDir string) (string, error) {
	initDir := filepath.Join(workdir, "postgres", "init")
	if err := os.MkdirAll(initDir, 0o755); err != nil {
		return "", fmt.Errorf("creating postgres init dir: %w", err)
	}

	rows := collectNpPluginsSeedRows(pluginDir)
	sql := buildNpPluginsSeedSQL(rows)

	out := filepath.Join(initDir, npPluginsSeedFilename)
	if err := os.WriteFile(out, []byte(sql), 0o644); err != nil {
		return "", fmt.Errorf("writing %s: %w", npPluginsSeedFilename, err)
	}
	return out, nil
}

// npPluginsSeedRow is one (name, tier) pair to be seeded.
type npPluginsSeedRow struct {
	Name string
	Tier string // "free" | "pro" | "internal"
}

// collectNpPluginsSeedRows walks pluginDir and returns one seed row per
// installed plugin. The result is sorted alphabetically by Name so the SQL
// output is deterministic across runs (important for `nself build`
// idempotency: re-running on the same install set must produce byte-equal
// SQL so docker-compose volumes don't perceive a change).
func collectNpPluginsSeedRows(pluginDir string) []npPluginsSeedRow {
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return nil
	}

	var rows []npPluginsSeedRow
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Skip plugins explicitly disabled by the operator.
		if _, err := os.Stat(filepath.Join(pluginDir, entry.Name(), ".disabled")); err == nil {
			continue
		}
		manifest := readPluginSeedManifest(pluginDir, entry.Name())
		if manifest == nil {
			continue
		}
		name := manifest.Name
		if name == "" {
			name = entry.Name()
		}
		rows = append(rows, npPluginsSeedRow{
			Name: name,
			Tier: pluginTier(manifest),
		})
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows
}

// pluginSeedManifest holds the manifest fields we read for tier resolution.
// We intentionally do NOT reuse pluginManifestMinimal from plugins.go: that
// type is keyed on build-time concerns (port, language, deps); we want a
// distinct view tied to seeding so future field changes here don't ripple.
type pluginSeedManifest struct {
	Name         string `json:"name"`
	IsCommercial bool   `json:"isCommercial"`
	LicenseType  string `json:"licenseType"`
	Visibility   string `json:"visibility"`
}

// readPluginSeedManifest reads plugin.json for tier-classification purposes.
// Returns nil if the file is missing or unparseable; callers must skip those
// plugins (we don't want to emit a row whose tier we can't determine).
func readPluginSeedManifest(pluginDir, name string) *pluginSeedManifest {
	path := filepath.Join(pluginDir, name, "plugin.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var m pluginSeedManifest
	// Permissive decode: unknown fields in plugin.json are ignored. We only
	// pull the four fields we need for tier classification.
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return &m
}

// pluginTier classifies a plugin manifest into 'free' / 'pro' / 'internal'.
//
// Rules (in priority order):
//  1. visibility == "internal"  -> "internal"
//  2. isCommercial == true OR licenseType == "pro" -> "pro"
//  3. otherwise -> "free"
//
// The np_plugins table CHECK constraint accepts only those three values
// (see internal/postgres/generator.go npPluginsInitSQL).
func pluginTier(m *pluginSeedManifest) string {
	if m == nil {
		return "free"
	}
	if strings.EqualFold(m.Visibility, "internal") {
		return "internal"
	}
	if m.IsCommercial || strings.EqualFold(m.LicenseType, "pro") {
		return "pro"
	}
	return "free"
}

// buildNpPluginsSeedSQL renders the seed SQL for the given rows. Always
// produces a stable, comment-led file even when rows is empty, so the
// generated file is present in every build.
func buildNpPluginsSeedSQL(rows []npPluginsSeedRow) string {
	var b strings.Builder
	b.WriteString("-- 05-np-plugins-seed.sql\n")
	b.WriteString("-- Generated by `nself build`. DO NOT EDIT.\n")
	b.WriteString("-- Idempotent: ON CONFLICT (name) DO NOTHING. Safe to re-run.\n")
	b.WriteString("-- Seeds one np_plugins row per plugin currently installed on disk.\n")
	b.WriteString("\n")

	if len(rows) == 0 {
		b.WriteString("-- No plugins installed; nothing to seed.\n")
		return b.String()
	}

	for _, r := range rows {
		fmt.Fprintf(&b,
			"INSERT INTO np_plugins (name, tier) VALUES ('%s', '%s') ON CONFLICT (name) DO NOTHING;\n",
			sqlEscape(r.Name), sqlEscape(r.Tier),
		)
	}
	return b.String()
}

// sqlEscape doubles single-quotes to safely embed a value inside a
// single-quoted SQL literal. Plugin names are validated upstream (alnum +
// hyphen) so this is belt-and-suspenders, not a primary defence.
func sqlEscape(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
