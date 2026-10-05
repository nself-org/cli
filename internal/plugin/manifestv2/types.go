// Package manifestv2 owns the plugin manifest v2 contract (contract:plugin.manifest).
//
// Purpose: one machine-checkable definition of plugin.json. The Go types here
// generate schemas/plugin-manifest.v2.schema.json (tools/schemagen), Load is the
// single reader (v2 natively, v1 through one normalizer), and Projection is the
// single derivation of the compatibility keys that released 1.4.x CLIs decode.
//
// Inputs: plugin.json bytes or a path. Outputs: a validated *Manifest or a
// coded error (E106, E111-E114). Constraints: layer L0 (Constitution 2.1): the
// standard library, internal/errs and internal/compat only; it never imports
// internal/plugin or cmd/. Released CLIs decode plugin.json permissively, so a v2
// key never reuses a v1 key with another JSON type (Epic D1, ADR 0009).
package manifestv2

// ManifestVersion is the value of manifest_version in a v2 file.
const ManifestVersion = 2

// Licence values (D2: bundle membership is not a manifest field).
const (
	LicenseFree     = "free"
	LicenseLicensed = "licensed"
)

// Maturity values (the lifecycle enum; the v1 `status` key is a compatibility key).
const (
	MaturityImplemented  = "implemented"
	MaturityExperimental = "experimental"
	MaturityScaffolded   = "scaffolded"
	MaturityPlanned      = "planned"
	MaturityDeferred     = "deferred"
)

// Deprecation states (deprecation.state, required when maturity is deferred).
const (
	StateDeprecated = "deprecated"
	StateEOL        = "eol"
)

// Service kinds.
const (
	KindCompose = "compose"
	KindCLI     = "cli"
	KindLibrary = "library"
)

// Command attribute values (CANON D5-D7 vocabulary).
const (
	SideRead        = "read"
	SideWrite       = "write"
	SideRemote      = "remote"
	SideDestructive = "destructive"
	OutDocument     = "document"
	OutStream       = "stream"
	OutInteractive  = "interactive"
	JSONNone        = "none"
	JSONLegacy      = "legacy"
	SurfaceAll      = "all"
	SurfaceCLIOnly  = "cli-only"
)

// Enum lists, shared by validation and the generated schema.
var (
	Licenses     = []string{LicenseFree, LicenseLicensed}
	Maturities   = []string{MaturityImplemented, MaturityExperimental, MaturityScaffolded, MaturityPlanned, MaturityDeferred}
	States       = []string{StateDeprecated, StateEOL}
	ServiceKinds = []string{KindCompose, KindCLI, KindLibrary}
	SideEffects  = []string{SideRead, SideWrite, SideRemote, SideDestructive}
	Outputs      = []string{OutDocument, OutStream, OutInteractive}
	JSONModes    = []string{JSONNone, JSONLegacy}
	Surfaces     = []string{SurfaceAll, SurfaceCLIOnly}
	Languages    = []string{"go", "rust", "typescript", "python", "bash"}
)

// Patterns, shared by validation and the generated schema.
const (
	NamePattern    = `^[a-z][a-z0-9-]*$`
	BinaryPattern  = `^nself-[a-z][a-z0-9-]*$`
	VersionPattern = `^v?[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?(\+[a-zA-Z0-9.]+)?$`
)

// ForbiddenKeys are rejected (E111) in a v2 file: they belong to the registry,
// the release pipeline or ADR 0008 (bundle membership), never to a manifest.
var ForbiddenKeys = []string{"author_public_key", "bundles", "checksum", "signature", "tier_pair"}

// Manifest is a manifest_version 2 plugin.json: the canonical v2 keys, the
// shared v1 keys (same name and JSON type as v1) and the generated
// compatibility keys.
type Manifest struct {
	ManifestVersion int    `json:"manifest_version"`
	Name            string `json:"name"`
	Version         string `json:"version"`
	Description     string `json:"description"`
	Category        string `json:"category"`
	// License is free or licensed (a v1 plugin.json carried an SPDX-style string
	// here; released CLIs only require it to be non-empty).
	License  string `json:"license"`
	Maturity string `json:"maturity"`
	// Installable defaults to true when absent (ADR 0009).
	Installable *bool     `json:"installable,omitempty"`
	Requires    *Requires `json:"requires,omitempty"`
	Service     *Service  `json:"service"`
	// Commands is nil for a plugin that adds no CLI command.
	Commands   *Commands   `json:"commands,omitempty"`
	Schema     *string     `json:"schema,omitempty"`
	Migrations *Migrations `json:"migrations,omitempty"`
	Seed       *Seed       `json:"seed,omitempty"`
	Env        *Env        `json:"env,omitempty"`
	Routes     []Route     `json:"routes,omitempty"`
	DocsURL    string      `json:"docs_url,omitempty"`

	Shared
	Compat
}

// Requires lists what must exist before the plugin can run.
type Requires struct {
	// Nself is a semver range for the CLI, e.g. ">=1.5.0".
	Nself string `json:"nself,omitempty"`
	// Plugins are plugin slugs.
	Plugins []string `json:"plugins,omitempty"`
	// PostgresExtensions are extension names checked at install (ADOPT).
	PostgresExtensions []string `json:"postgres_extensions,omitempty"`
}

// Service describes how the plugin runs.
type Service struct {
	Kind        string  `json:"kind"`
	Compose     *string `json:"compose,omitempty"`
	Image       *string `json:"image,omitempty"`
	Port        *int    `json:"port,omitempty"`
	Healthcheck *string `json:"healthcheck,omitempty"`
}

// Migrations declares boot-applied SQL (ADR 0010).
type Migrations struct {
	Dir   string `json:"dir"`
	Apply string `json:"apply"`
}

// Seed is an idempotent argv run inside the plugin's service.
type Seed struct {
	Command []string `json:"command"`
}

// Env lists environment variable names.
type Env struct {
	Required []string `json:"required,omitempty"`
	Optional []string `json:"optional,omitempty"`
}

// Route exposes a plugin port through nginx.
type Route struct {
	Path         string `json:"path"`
	UpstreamPort int    `json:"upstream_port"`
}

// PostgresExtensions returns requires.postgres_extensions, empty when absent
// (the install check of P7-ADOPT-06 reads it).
func PostgresExtensions(m *Manifest) []string {
	if m == nil || m.Requires == nil {
		return []string{}
	}
	return append([]string{}, m.Requires.PostgresExtensions...)
}
