// Package catalog is the CLI side of contract:plugins.catalog v1 (P7-PLUG-09):
// the one JSON document at https://plugins.nself.org/catalog.json that lists
// every plugin of both tiers, its commands and its bundles.
//
// Purpose: fetch the catalog with an ETag cache and an offline fallback, decode
// it strictly enough to reject a broken publish, and answer the questions the
// CLI asks of it: search, lookup by slug, which plugin provides a command.
// Inputs: the catalog URL and a cache directory (default ~/.nself/cache/plugins).
// Outputs: *Catalog values, the Source they came from, and E115/E116 errors.
// Constraints: catalog data is display-only. Nothing here installs anything;
// installs still verify the checksum and signature (P7-PLUG-23). All HTTP goes
// through internal/httptimeout (Constitution 2.2). The unknown-command path
// reads the cache only and never touches the network.
//
// Layer: L2 (internal/plugin/**). Imports errs and httptimeout only.
package catalog

// SchemaVersion is the only catalog schema_version this package decodes.
const SchemaVersion = 1

// Tier values as the catalog writes them.
const (
	TierFree     = "free"
	TierLicensed = "licensed"
)

// Catalog is the decoded catalog document. Fields the CLI does not read
// (counts) are skipped; unknown fields are ignored so an additive publish does
// not break an older CLI.
type Catalog struct {
	SchemaVersion int               `json:"schema_version"`
	Generated     string            `json:"_generated,omitempty"`
	GeneratedFrom GeneratedFrom     `json:"generated_from"`
	Plugins       []Entry           `json:"plugins"`
	Bundles       map[string]Bundle `json:"bundles,omitempty"`
}

// GeneratedFrom records the source refs and time of the publish.
type GeneratedFrom struct {
	PluginsRef  string `json:"plugins_ref"`
	BundlesRef  string `json:"bundles_ref"`
	GeneratedAt string `json:"generated_at"`
}

// Entry is one plugin at one tier. A tier-pair slug appears once per tier.
type Entry struct {
	Slug        string    `json:"slug"`
	Tier        string    `json:"tier"`
	Version     string    `json:"version"`
	Description string    `json:"description"`
	Category    string    `json:"category"`
	Maturity    string    `json:"maturity"`
	Installable bool      `json:"installable"`
	ServiceKind string    `json:"service_kind"`
	Commands    *Commands `json:"commands"`
	DocsURL     *string   `json:"docs_url"`
	Bundles     []string  `json:"bundles"`
	TierPair    bool      `json:"tier_pair"`
}

// Commands is the CLI command a plugin provides (null when it provides none).
type Commands struct {
	Command     string   `json:"command"`
	Subcommands []string `json:"subcommands"`
}

// Bundle is one bundle: display name, tier and member plugin slugs.
type Bundle struct {
	Display string   `json:"display"`
	Tier    string   `json:"tier"`
	Plugins []string `json:"plugins"`
}
