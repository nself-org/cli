package plugin

import "context"

// RegistryClient abstracts plugin registry HTTP operations for testability.
type RegistryClient interface {
	Fetch(ctx context.Context) (*Registry, error)
	GetPlugin(ctx context.Context, name string) (*PluginManifest, error)
}

// LicenseClient abstracts license validation for testability.
type LicenseClient interface {
	Validate(ctx context.Context, key string) (bool, error)
}

// Registry represents the full plugin registry response.
type Registry struct {
	Plugins []PluginManifest
	// FetchedAt is the registry-wide snapshot timestamp reported by the live
	// endpoint's top-level "fetchedAt" field (CLI-R16). Empty for registries
	// that don't send it (e.g. the GitHub raw fallback).
	FetchedAt string
}

// EnvVar describes an environment variable required or used by a plugin.
type EnvVar struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
	Default     string `json:"default,omitempty"`
}

// CLICommand describes a CLI command provided by a plugin.
type CLICommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// SystemDependency describes a system-level dependency for a plugin.
type SystemDependency struct {
	Name          string `json:"name"`
	Verify        string `json:"verify"`
	MinVersion    string `json:"minVersion,omitempty"`
	Apt           string `json:"apt,omitempty"`
	Brew          string `json:"brew,omitempty"`
	CustomInstall string `json:"custom_install,omitempty"`
}

// SystemDependencies groups required and recommended system deps.
type SystemDependencies struct {
	Required    []SystemDependency `json:"required,omitempty"`
	Recommended []SystemDependency `json:"recommended,omitempty"`
}

// DeprecationBlock carries the required deprecation metadata when a plugin's
// status is "deprecated". All five fields must be present (S58-T02).
// announcedDate and eolDate are ISO 8601 date strings (YYYY-MM-DD).
// replacedBy is the name of the replacement plugin (optional but recommended).
// migrationGuide is a URL that must resolve with HTTP 200 at CI time.
// migrationScript is a repo-relative path to an automated migration script (optional).
type DeprecationBlock struct {
	AnnouncedDate   string `json:"announcedDate"`
	EOLDate         string `json:"eolDate"`
	ReplacedBy      string `json:"replacedBy,omitempty"`
	MigrationGuide  string `json:"migrationGuide"`
	MigrationScript string `json:"migrationScript,omitempty"`
}

// MultiApp describes multi-tenancy configuration for a plugin.
type MultiApp struct {
	Supported       bool   `json:"supported"`
	IsolationColumn string `json:"isolationColumn,omitempty"`
	PKStrategy      string `json:"pkStrategy,omitempty"`
	DefaultValue    string `json:"defaultValue,omitempty"`
}

// PluginGraphQLEntityKey describes a single Apollo Federation entity type
// that a plugin subgraph owns. Used for cross-subgraph entity resolution.
type PluginGraphQLEntityKey struct {
	// Type is the GraphQL type name (e.g. "User").
	Type string `json:"type" yaml:"type"`
	// Key is the @key field used for entity resolution (e.g. "id").
	Key string `json:"key" yaml:"key"`
}

// PluginGraphQLBlock is the graphql: block in a plugin manifest.
// When Enabled is true and NSELF_FEDERATION=true, nself build registers
// this plugin as a subgraph in the Apollo Router supergraph (G05).
type PluginGraphQLBlock struct {
	// Enabled must be true for this plugin to register as a subgraph.
	Enabled bool `json:"enabled" yaml:"enabled"`
	// SubgraphName is the unique lowercase subgraph identifier.
	// Must not be "hasura" (reserved). Format: [a-z][a-z0-9_]*.
	SubgraphName string `json:"subgraph_name" yaml:"subgraph_name"`
	// SubgraphURL is the HTTP endpoint serving the subgraph SDL.
	// ${PORT} is substituted with the plugin's declared Port field.
	SubgraphURL string `json:"subgraph_url" yaml:"subgraph_url"`
	// SchemaPath is a repo-relative path to the static SDL file.
	// Used by rover supergraph compose. Optional when SubgraphURL is live.
	SchemaPath string `json:"schema_path,omitempty" yaml:"schema_path,omitempty"`
	// Entities lists Apollo Federation entity types this subgraph owns.
	Entities []PluginGraphQLEntityKey `json:"entities,omitempty" yaml:"entities,omitempty"`
}

// PluginManifest describes a single plugin, parsed from plugin.json.
type PluginManifest struct {
	// Required fields
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Category    string `json:"category"`
	License     string `json:"license"`

	// Optional metadata
	Author               string   `json:"author,omitempty"`
	Homepage             string   `json:"homepage,omitempty"`
	Repository           string   `json:"repository,omitempty"`
	Tags                 []string `json:"tags,omitempty"`
	IsCommercial         bool     `json:"isCommercial,omitempty"`
	LicenseType          string   `json:"licenseType,omitempty"`
	RequiredEntitlements []string `json:"requiredEntitlements,omitempty"`
	RequiresLicense      bool     `json:"requires_license,omitempty"`
	MinNselfVersion      string   `json:"minNselfVersion,omitempty"`
	MinNodeVersion       string   `json:"minNodeVersion,omitempty"`
	ArchSupport          []string `json:"arch_support,omitempty"`

	// Implementation
	Language   string `json:"language,omitempty"`
	Runtime    string `json:"runtime,omitempty"`
	Port       int    `json:"port,omitempty"`
	EntryPoint string `json:"entryPoint,omitempty"`
	CLI        string `json:"cli,omitempty"`
	// PluginType distinguishes a plugin that adds a CLI command ("cli") from
	// one that runs as a service. Only a "cli" plugin gets its binary published
	// into the directory ProxyCommand searches — see cli_binary.go.
	PluginType string `json:"pluginType,omitempty"`
	// BinaryName overrides the nself-<name> convention for the published
	// binary. ProxyCommand resolves nself-<command>, so this should only differ
	// when the command name differs from the plugin name.
	BinaryName     string `json:"binaryName,omitempty"`
	HealthEndpoint string `json:"health_endpoint,omitempty"`
	PackageManager string `json:"packageManager,omitempty"`
	Framework      string `json:"framework,omitempty"`

	// Database
	Tables []string `json:"tables,omitempty"`
	Views  []string `json:"views,omitempty"`

	// API
	APIEndpoints APIEndpointList `json:"apiEndpoints,omitempty"`
	// WebhookNames, not []string: real manifests use both an array of event
	// names and an object keyed by event name. See webhooks.go.
	Webhooks    WebhookNames `json:"webhooks,omitempty"`
	CLICommands []CLICommand `json:"cliCommands,omitempty"`

	// Environment
	EnvVars EnvVarList `json:"envVars,omitempty"`

	// Dependencies
	Dependencies         []string           `json:"dependencies,omitempty"`
	OptionalDependencies []string           `json:"optionalDependencies,omitempty"`
	SystemDependencies   SystemDependencies `json:"systemDependencies,omitempty"`

	// Inter-plugin communication contract (S43-T17).
	// Consumes lists the plugin names whose HTTP APIs this plugin may call via
	// X-Source-Plugin. Every entry must be installed for this plugin to work
	// correctly; install-time validation enforces this.
	// Provides lists the API services this plugin exposes for other plugins to
	// consume. Both fields use plugin name format (lowercase-with-hyphens).
	Consumes []string `json:"consumes,omitempty"`
	Provides []string `json:"provides,omitempty"`

	// Multi-tenancy
	MultiApp MultiApp `json:"multiApp,omitempty"`

	// Permissions
	Permissions PermissionSet `json:"permissions,omitempty"`

	// Compatibility
	Compat *CompatBlock `json:"compat,omitempty"`

	// Registry-specific fields (not in plugin.json, populated by registry)
	Tier     string `json:"tier,omitempty"`
	Checksum string `json:"checksum,omitempty"`

	// TierPair marks a genuine free/pro pair of the SAME product sharing one
	// slug (e.g. cron, notify) — set true in BOTH the free and pro registry
	// entries by the plugins/plugins-pro registry authors. Install-time
	// resolution (tier_resolve.go) uses this to tell a real tier pair from an
	// unrelated registry-data collision, which is a hard error instead
	// (OWNER-ACTIONS.md item 15).
	TierPair bool `json:"tier_pair,omitempty"`

	// Bundles lists the bundles.json slugs this specific registry entry
	// belongs to (e.g. cron's pro entry: ["claw"]). Used by tier_resolve.go
	// to decide entitlement for a TierPair's pro entry via
	// license.BundleEntitled. Empty for free entries and for pro plugins sold
	// standalone rather than through a bundle.
	Bundles []string `json:"bundles,omitempty"`

	// PublishStatus is the plugin's lifecycle status from the registry (S58-T01).
	// Valid values: "experimental" | "planned" | "beta" | "stable" | "deprecated" | "eol"
	// Missing status defaults to "stable" for backwards compatibility (CR-A).
	// "planned"     — not yet installable; rejected at install time.
	// "experimental"— early-access; installs with a prominent warning.
	// "beta"        — pre-stable; installs with a warning.
	// "stable"      — default production-ready state; no warning.
	// "deprecated"  — install warns and offers replacedBy alternative if set.
	//                 Requires a Deprecation block in the manifest.
	// "eol"         — install blocked by default; --allow-eol override required.
	PublishStatus string `json:"status,omitempty"`

	// Deprecation carries the required metadata when status == "deprecated". (S58-T02)
	// All five fields are required when the block is present.
	Deprecation *DeprecationBlock `json:"deprecation,omitempty"`

	// GraphQL carries the Apollo Federation subgraph registration for this
	// plugin. When graphql.enabled is true and NSELF_FEDERATION=true, the
	// build pipeline includes this plugin as a subgraph in the Apollo Router
	// supergraph. The field is ignored in non-federation deployments (G05).
	GraphQL *PluginGraphQLBlock `json:"graphql,omitempty"`

	// MaxNselfVersion is the last CLI version this plugin is compatible with. (S58-T06)
	// Empty means "no upper bound". compat-check reads this field.
	MaxNselfVersion string `json:"maxNselfVersion,omitempty"`

	// AuthorPublicKey is the hex-encoded Ed25519 public key of the plugin
	// publisher, pinned in the registry. Used to verify Signature.
	AuthorPublicKey string `json:"author_public_key,omitempty"`

	// Signature is the hex-encoded Ed25519 signature of the tarball checksum.
	// Format: Ed25519Sign(privateKey, sha256(tarball)).
	// Pinned in registry alongside AuthorPublicKey.
	Signature string `json:"signature,omitempty"`

	// UpdatedAt is a per-plugin freshness timestamp (CLI-R16). The live
	// plugins.nself.org registry does not send this field as of 2026-08-23 —
	// only a registry-wide "fetchedAt" (see Registry.FetchedAt /
	// RegistryFetchedAt). This field is plumbing: `nself plugin list
	// --detailed` displays it when present (registry entry or local
	// plugin.json) and shows nothing invented when it is not.
	UpdatedAt string `json:"updated_at,omitempty"`

	// PlatformChecksums holds one SHA-256 checksum (lowercase hex, no
	// "sha256:" prefix) per per-platform binary tarball, keyed by the exact
	// platform string PlatformArch() returns (darwin-arm64, darwin-amd64,
	// linux-amd64, linux-arm64, windows-amd64). Populated from the registry's
	// nested `checksums.platforms` object.
	//
	// Checksum above is ALWAYS the source tarball's checksum, never a
	// platform one — the two packages have different bytes, so one flat
	// field can never validate both (FIX-CLI-plugins-83: a platform tarball
	// downloaded via binaryPluginDownloadURL was being checked against
	// Checksum, which could never match). downloadPluginPackageForTier
	// reports which artifact it fetched; installLocked picks the matching
	// entry from this map, or Checksum for the source artifact — see
	// verifyChecksum's caller in installer_locked.go.
	//
	// Only present for a plugin with a binaryName. A plugin whose release
	// predates this field (or a platform the release never built a checksum
	// for) has no entry here — see downloadPluginPackageForTier's doc
	// comment for what happens then.
	PlatformChecksums map[string]string `json:"platform_checksums,omitempty"`
}
