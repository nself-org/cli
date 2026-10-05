package plugin

// registry_catalog.go: the registry entry's additive `catalog` block.
//
// Purpose: P7-PLUG-07's generator writes catalog.requires from manifest v2
// requires; the install reads postgres_extensions from it (P7-ADOPT-06).
// Inputs: the decoded registry entry. Outputs: the extension names, or nil.
// Constraints: additive only; released CLIs ignore the `catalog` key, and a
// registry without it decodes exactly as before.

// pluginCatalog is the subset of the catalog block this package reads.
type pluginCatalog struct {
	Requires *struct {
		PostgresExtensions []string `json:"postgres_extensions,omitempty"`
	} `json:"requires,omitempty"`
}

// postgresExtensions returns catalog.requires.postgres_extensions or nil.
func (c *pluginCatalog) postgresExtensions() []string {
	if c == nil || c.Requires == nil {
		return nil
	}
	return c.Requires.PostgresExtensions
}

// catalogFor wraps extension names back into a catalog block for the cache.
func catalogFor(exts []string) *pluginCatalog {
	if len(exts) == 0 {
		return nil
	}
	c := &pluginCatalog{}
	c.Requires = &struct {
		PostgresExtensions []string `json:"postgres_extensions,omitempty"`
	}{PostgresExtensions: exts}
	return c
}

// RequiresPostgresExtensions returns the Postgres extensions the plugin needs,
// from manifest v2 requires.postgres_extensions (a v2 plugin.json) or the
// registry entry's catalog.requires. Empty when none are declared.
func (m *PluginManifest) RequiresPostgresExtensions() []string {
	if m == nil {
		return nil
	}
	return append([]string(nil), m.PostgresExtensions...)
}
