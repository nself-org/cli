package model

// Catalog is catalog.json (contract:plugins.catalog v1). Plugins are sorted by
// (slug, tier); a tier-pair slug appears once per tier and counts once.
type Catalog struct {
	Generated     string                   `json:"_generated"`
	SchemaVersion int                      `json:"schema_version"`
	GeneratedFrom GeneratedFrom            `json:"generated_from"`
	Plugins       []CatalogPlugin          `json:"plugins"`
	Bundles       map[string]CatalogBundle `json:"bundles"`
	Counts        Counts                   `json:"counts"`
}

// CatalogPlugin is one row of the catalog. Nullable keys are pointers and are
// always written (null), so a consumer never has to tell absent from empty.
type CatalogPlugin struct {
	Slug             string     `json:"slug"`
	Tier             string     `json:"tier"`
	Version          string     `json:"version"`
	Description      string     `json:"description"`
	Category         string     `json:"category"`
	Maturity         string     `json:"maturity"`
	Installable      bool       `json:"installable"`
	ServiceKind      string     `json:"service_kind"`
	Commands         *Commands  `json:"commands"`
	Schema           *string    `json:"schema"`
	Requires         *Requires  `json:"requires"`
	DocsURL          *string    `json:"docs_url"`
	Bundles          []string   `json:"bundles"`
	TierPair         bool       `json:"tier_pair"`
	Checksum         string     `json:"checksum"`
	ReleaseSignature *Signature `json:"release_signature"`
	TarballURL       string     `json:"tarball_url"`
}

// CatalogBundle is one bundle of bundles.json: tier is free or licensed.
type CatalogBundle struct {
	Display string   `json:"display"`
	Tier    string   `json:"tier"`
	Plugins []string `json:"plugins"`
}

// Bundles is bundles.json, the membership source of truth (ADR 0008). Only the
// keys the generator reads are decoded; prices and pages are not catalog data.
type Bundles struct {
	SchemaVersion string                 `json:"schema_version"`
	Bundles       map[string]BundleEntry `json:"bundles"`
}

// BundleEntry is one bundle of bundles.json. Tier is `free` or `paid` there.
type BundleEntry struct {
	Display string   `json:"display"`
	Tier    string   `json:"tier"`
	Plugins []string `json:"plugins"`
}

// Counts is counts.json (contract:plugins.counts v1). The wire key `pro` is kept
// for the vendored consumer (internal/plugin/count). `generated_at` and
// `sources` of the old script are replaced by generated_from.
type Counts struct {
	Generated     string        `json:"_generated"`
	SchemaVersion int           `json:"schema_version"`
	GeneratedFrom GeneratedFrom `json:"generated_from"`
	Free          TierCount     `json:"free"`
	Pro           TierCount     `json:"pro"`
	Overlap       Overlap       `json:"overlap"`
	Totals        Totals        `json:"totals"`
	Advertised    int           `json:"advertised"`
}

// TierCount is the entries and installable breakdown for one registry.
type TierCount struct {
	Entries        int      `json:"entries"`
	Installable    int      `json:"installable"`
	NonInstallable []string `json:"nonInstallable"`
}

// Overlap lists slugs present in both registries: one two-tier product each,
// counted once. tierPairs and duplicates hold the same slugs.
type Overlap struct {
	SharedSlugs []string `json:"sharedSlugs"`
	TierPairs   []string `json:"tierPairs"`
	Duplicates  []string `json:"duplicates"`
}

// Totals is the combined view after de-duplication.
type Totals struct {
	Entries     int `json:"entries"`
	Installable int `json:"installable"`
}
