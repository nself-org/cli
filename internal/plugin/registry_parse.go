package plugin

// registry_parse.go — registry JSON envelope types and endpoint/dependency parsing.
//
// Purpose: define the raw registry JSON shape and parse its API endpoint and dependency sub-structures, used by parseRegistryJSON in registry_json.go, split out of registry.go for file size.
// Inputs: raw JSON bytes fetched by Fetch/fetchFromURL in registry.go.
// Outputs: registryEnvelope/pluginEntry values with parsed endpoints and dependencies.
// Constraints: pure move from registry.go (CLI-R12 Batch F); no behaviour change.

import (
	"encoding/json"
	"strings"
)

// registryEnvelope is an intermediate type for unmarshaling the two
// registry formats. The Plugins field is kept as raw JSON so we can
// detect whether it is an object or an array.
type registryEnvelope struct {
	Version     string `json:"version"`
	LastUpdated string `json:"lastUpdated"`
	GeneratedAt string `json:"generated_at"`
	// FetchedAt is the live registry's snapshot timestamp (RFC3339), present
	// on the plugins.nself.org Cloudflare Worker response as of 2026-08-23
	// (CLI-R16 inspection: `curl https://plugins.nself.org/registry.json`).
	// It is registry-wide, not per-plugin — the live payload does not carry
	// a per-plugin last-updated field. PluginManifest.UpdatedAt (below) is
	// plumbing for if/when the registry adds one; it is not populated by
	// today's live data.
	FetchedAt string          `json:"fetchedAt"`
	Tier      string          `json:"tier"`
	Plugins   json.RawMessage `json:"plugins"`
}

// pluginImplementation holds the nested implementation block present in the
// Cloudflare Worker registry format (plugins.nself.org).
type pluginImplementation struct {
	Language       string `json:"language"`
	Runtime        string `json:"runtime"`
	DefaultPort    int    `json:"defaultPort"`
	EntryPoint     string `json:"entryPoint"`
	CLI            string `json:"cli"`
	PackageManager string `json:"packageManager"`
	Framework      string `json:"framework"`

	// PluginType and BinaryName were missing here, and their absence made the
	// whole install-to-proxy bridge a no-op for anything installed from the
	// registry — which is every real install.
	//
	// Registry entries declare these inside the implementation block. Parsing
	// dropped them, so PluginManifest.PluginType arrived empty, cliBinaryName
	// returned "", and linkCLIBinary did nothing. A CLI plugin installed
	// "successfully" and its command did not exist.
	PluginType string `json:"pluginType"`
	BinaryName string `json:"binaryName"`
}

// pluginEndpointEntry holds the object form of an API endpoint as returned
// by the Cloudflare Worker registry: {"method":"GET","path":"/v1/foo","description":"..."}.
type pluginEndpointEntry struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Description string `json:"description"`
}

// pluginChecksumsEntry holds the registry's nested `checksums` object.
// `sha256` duplicates the flat `checksum` field (source tarball, kept for
// backwards compatibility with older registry readers); `platforms` carries
// one checksum per per-platform binary tarball, keyed by the exact platform
// string internal/plugin/arch.go's PlatformArch() returns. Only present for
// a plugin with a binaryName — see PluginManifest.PlatformChecksums.
type pluginChecksumsEntry struct {
	SHA256    string            `json:"sha256,omitempty"`
	Platforms map[string]string `json:"platforms,omitempty"`
}

// pluginEntry matches the fields present in the array-format (pro)
// registry as well as the object-format (free) registry.
// APIEndpoints is kept as json.RawMessage because the live registry returns
// it as an array of objects while older/local registries use an array of
// strings. We normalise both into []string during entryToManifest conversion.
type pluginEntry struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Tier        string `json:"tier"`
	License     string `json:"license"`
	LicenseType string `json:"licenseType"`
	Repository  string `json:"repository"`
	Checksum    string `json:"checksum"`
	// Checksums is the nested form carrying per-platform binary tarball
	// checksums (checksums.platforms) alongside the source-tarball checksum
	// (checksums.sha256, a duplicate of the flat Checksum field above). See
	// pluginChecksumsEntry and PluginManifest.PlatformChecksums.
	Checksums       *pluginChecksumsEntry `json:"checksums,omitempty"`
	DownloadURL     string                `json:"download_url"`
	RequiresLicense bool                  `json:"requires_license"`
	Tags            []string              `json:"tags"`
	Tables          []string              `json:"tables,omitempty"`
	Port            int                   `json:"port,omitempty"`
	// TierPair and Bundles support install-time tier resolution for a slug
	// served twice (free + pro) as one product — see PluginManifest's doc
	// comments (interfaces.go) and tier_resolve.go.
	TierPair bool     `json:"tier_pair,omitempty"`
	Bundles  []string `json:"bundles,omitempty"`
	// Dependencies is raw JSON because the registry format is not stable.
	// Accepted shapes:
	//   ["redis","auth"]                            (legacy string array)
	//   {"required":[...], "optional":[...]}        (Cloudflare Worker schema)
	//   {"plugins":[...]}                           (plugin-only dependencies)
	//   {"npm":[...], "system":[...], "python":[...]} (system/package deps — ignored
	//                                                  here; system deps are handled
	//                                                  separately by the loader)
	Dependencies json.RawMessage `json:"dependencies,omitempty"`
	// PublishStatus is "stable", "beta", or "planned". Planned plugins are
	// rejected at install time; beta plugins install with a warning.
	PublishStatus string `json:"status,omitempty"`
	// AuthorPublicKey is the hex-encoded Ed25519 public key pinned in the
	// registry for signature verification (T09).
	AuthorPublicKey string `json:"author_public_key,omitempty"`
	// Signature is the hex-encoded Ed25519 signature of the tarball checksum.
	Signature string `json:"signature,omitempty"`
	// Implementation may appear as a nested object (Cloudflare Worker format)
	// or as flat fields (older registry format).
	Implementation *pluginImplementation `json:"implementation,omitempty"`

	// Flat forms of the implementation fields, accepted so a registry entry can
	// declare them either way. Real registry entries use the nested form; the
	// cache writes the flat form, and both have to round-trip.
	Language   string `json:"language,omitempty"`
	Runtime    string `json:"runtime,omitempty"`
	PluginType string `json:"pluginType,omitempty"`
	BinaryName string `json:"binaryName,omitempty"`

	// CLICommands lists every command a plugin provides. Dropping it meant a
	// plugin declaring two commands had only one binary published, so the
	// second command stayed dead after a successful install.
	CLICommands []CLICommand `json:"cliCommands,omitempty"`

	// The rest of the manifest. These were absent, so the cache round-trip
	// silently reset them — see TestRegistryRoundTripLosesNoField for why that
	// class of omission keeps producing production bugs.
	Author               string              `json:"author,omitempty"`
	Homepage             string              `json:"homepage,omitempty"`
	IsCommercial         bool                `json:"isCommercial,omitempty"`
	RequiredEntitlements []string            `json:"requiredEntitlements,omitempty"`
	MinNselfVersion      string              `json:"minNselfVersion,omitempty"`
	MaxNselfVersion      string              `json:"maxNselfVersion,omitempty"`
	MinNodeVersion       string              `json:"minNodeVersion,omitempty"`
	ArchSupport          []string            `json:"arch_support,omitempty"`
	EntryPoint           string              `json:"entryPoint,omitempty"`
	CLI                  string              `json:"cli,omitempty"`
	HealthEndpoint       string              `json:"health_endpoint,omitempty"`
	PackageManager       string              `json:"packageManager,omitempty"`
	Framework            string              `json:"framework,omitempty"`
	Views                []string            `json:"views,omitempty"`
	MultiApp             MultiApp            `json:"multiApp,omitempty"`
	Deprecation          *DeprecationBlock   `json:"deprecation,omitempty"`
	GraphQL              *PluginGraphQLBlock `json:"graphql,omitempty"`
	// APIEndpoints is raw JSON because the registry format is not stable:
	// the live registry returns objects; older registries return strings.
	APIEndpoints json.RawMessage `json:"apiEndpoints,omitempty"`
	// Compat holds CLI and service version constraints.
	Compat *CompatBlock `json:"compat,omitempty"`
	// UpdatedAt is a per-plugin freshness timestamp (CLI-R16). Not present on
	// the live plugins.nself.org registry as of 2026-08-23 — kept as forward
	// plumbing so `nself plugin list --detailed` picks it up the moment the
	// registry starts sending it, without another code change.
	UpdatedAt string `json:"updated_at,omitempty"`
}

// EffectiveStatus normalises a plugin's raw registry `status` value for every
// status-driven decision: an absent (empty) status defaults to "stable" — see
// pluginEntry.PublishStatus / PluginManifest.PublishStatus's doc comment
// ("Missing status defaults to stable for backwards compatibility"). Before
// FIX-CLI-6 every status gate compared the raw field directly against the
// literal string "stable", so "" and "stable" were treated inconsistently:
// the lifecycle switch in installLocked already special-cased both the same
// way, but verifyChecksum/verifyPluginSignature's "missing value" branch did
// not, leaving all 130 (of 177, as of 2026-09-04) registry entries without an
// explicit status effectively unable to ever hit the "stable" hard-fail path.
// Every status-driven gate must compare against EffectiveStatus(status), not
// the raw field, so "" and an explicit "stable" are never inconsistent again.
func EffectiveStatus(status string) string {
	if status == "" {
		return "stable"
	}
	return status
}

// parseAPIEndpoints converts the raw apiEndpoints JSON value from either the
// string-array format (["path1","path2"]) or the object-array format
// ([{"method":"GET","path":"/v1/foo"},...]) into a normalised []string.
// Unknown/null values yield nil without error.
func parseAPIEndpoints(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "null" || trimmed == "" {
		return nil
	}

	// Try string array first (legacy format).
	var strs []string
	if err := json.Unmarshal(raw, &strs); err == nil {
		return strs
	}

	// Fall back to object array (Cloudflare Worker / live registry format).
	var objs []pluginEndpointEntry
	if err := json.Unmarshal(raw, &objs); err != nil {
		return nil
	}
	out := make([]string, 0, len(objs))
	for _, ep := range objs {
		if ep.Path != "" {
			out = append(out, ep.Path)
		}
	}
	return out
}

// normalizePlatformChecksums copies a registry's checksums.platforms map,
// stripping an optional "sha256:" prefix from each value. The plugins repo
// writes the sibling checksums.sha256 field with that prefix (see
// release-tarballs.yml's backfill step); platform checksums are documented
// to be written as plain hex, but stripping a stray prefix defensively here
// costs nothing and avoids a checksum that LOOKS present but can never
// match verifyChecksum's raw hex comparison — which would otherwise surface
// as an install-time checksum mismatch instead of the registry data error it
// actually is. Returns nil for an empty/nil input, matching the omitempty
// contract used throughout this package.
func normalizePlatformChecksums(raw map[string]string) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]string, len(raw))
	for platform, checksum := range raw {
		out[platform] = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(checksum)), "sha256:")
	}
	return out
}

// parseDependencies extracts the list of plugin-name dependencies from the
// raw `dependencies` JSON value. The registry has shipped several shapes over
// time, so we accept all of them and ignore non-plugin keys (npm/system/python)
// since system-level deps are handled by a separate loader step.
//
// Accepted shapes:
//
//	null / missing                          → nil
//	["redis","auth"]                        → ["redis","auth"]
//	{"required":[...], "optional":[...]}    → required ++ optional
//	{"plugins":[...]}                       → plugins
//	{"npm":[...]} / {"system":[...]}        → nil (system deps, not plugin deps)
//
// Unknown shapes return nil rather than erroring — a stricter manifest schema
// is the right place to reject malformed registries; this parser exists so a
// schema drift never bricks `nself plugin install`.
func parseDependencies(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil
	}

	// Legacy: plain string array.
	var strs []string
	if err := json.Unmarshal(raw, &strs); err == nil {
		if len(strs) == 0 {
			return nil
		}
		return strs
	}

	// Object form. Pull out plugin-name keys; ignore system/package-manager keys.
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	out := []string{}
	for _, key := range []string{"required", "plugins", "optional"} {
		if v, ok := obj[key]; ok {
			var list []string
			if err := json.Unmarshal(v, &list); err == nil {
				out = append(out, list...)
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
