package plugin

import (
	"encoding/json"

	"github.com/nself-org/cli/internal/plugin/manifestv2"
)

// Adapter from manifest v2 to the runtime PluginManifest (P7-PLUG-01).
//
// Purpose: a plugin.json with manifest_version 2 is read from its v2 fields,
// not through the v1 decoder: manifestv2.Parse strict-decodes it, validates it
// and proves its compatibility keys equal the projection (E112 otherwise, so a
// file edited after the gate is never used silently). The runtime keeps its one
// PluginManifest type; this file fills it from the canonical and shared v2
// keys, then runs the same validateManifest every v1 manifest goes through
// (permission allowlist included).
//
// Inputs: plugin.json bytes. Outputs: *PluginManifest. Constraints: v1 files
// (manifest_version absent, null, 0, "1", 1 or 1.0) are not handled here and parse exactly as before.

// LoadManifest reads and validates the plugin.json at path, v1 or v2.
func LoadManifest(path string) (*PluginManifest, error) { return parseManifest(path) }

// parseManifestV2 handles a manifest_version other than absent or 1. ok is
// false for a v1 document, which the caller decodes as before.
func parseManifestV2(data []byte) (m *PluginManifest, ok bool, err error) {
	var probe struct {
		Version json.RawMessage `json:"manifest_version"`
	}
	if json.Unmarshal(data, &probe) != nil {
		return nil, false, nil
	}
	if manifestv2.IsV1Version(probe.Version) {
		return nil, false, nil
	}
	v2, err := manifestv2.Parse(data)
	if err != nil {
		return nil, true, err
	}
	pm := fromManifestV2(v2)
	if err := validateManifest(pm); err != nil {
		return nil, true, err
	}
	return pm, true, nil
}

// fromManifestV2 builds the runtime manifest from the v2 fields. Registry-only
// fields (checksum, tier_pair, bundles, signature) stay empty: a v2 file cannot
// carry them.
func fromManifestV2(v *manifestv2.Manifest) *PluginManifest {
	m := &PluginManifest{
		Name: v.Name, Version: v.Version, Description: v.Description, Category: v.Category, License: licenseText(v),
		Author: v.Author, Homepage: v.Homepage, Repository: v.Repository, Tags: v.Tags,
		IsCommercial: v.IsCommercial, LicenseType: v.LicenseType, RequiredEntitlements: v.RequiredEntitlements,
		RequiresLicense: v.RequiresLicense, MinNselfVersion: v.MinNselfVersion, MinNodeVersion: v.MinNodeVersion,
		ArchSupport: v.ArchSupport, Language: v.Language, Runtime: v.Runtime, Port: v.Port,
		EntryPoint: v.EntryPoint, CLI: v.CLI, PluginType: v.PluginType, BinaryName: v.BinaryName,
		HealthEndpoint: v.HealthEndpoint, PackageManager: v.PackageManager, Framework: v.Framework,
		Tables: v.Tables, Views: v.Views, APIEndpoints: APIEndpointList(v.APIEndpoints), Webhooks: WebhookNames(v.Webhooks),
		Dependencies: v.Dependencies, OptionalDependencies: v.OptionalDependencies,
		Consumes: v.Consumes, Provides: v.Provides, Tier: v.Tier, PublishStatus: v.Status,
		MaxNselfVersion: v.MaxNselfVersion, UpdatedAt: v.UpdatedAt, PlatformChecksums: v.PlatformChecksums,
		Permissions: permissionsFromV2(v.Permissions),
	}
	for _, c := range v.CLICommands {
		m.CLICommands = append(m.CLICommands, CLICommand(c))
	}
	for _, e := range v.EnvVars {
		m.EnvVars = append(m.EnvVars, EnvVar(e))
	}
	if sd := v.SystemDependencies; sd != nil {
		for _, d := range sd.Required {
			m.SystemDependencies.Required = append(m.SystemDependencies.Required, SystemDependency(d))
		}
		for _, d := range sd.Recommended {
			m.SystemDependencies.Recommended = append(m.SystemDependencies.Recommended, SystemDependency(d))
		}
	}
	if v.MultiApp != nil {
		m.MultiApp = MultiApp(*v.MultiApp)
	}
	if c := v.CompatBlock; c != nil {
		m.Compat = &CompatBlock{Nself: c.Nself, Requires: c.Requires}
	}
	if d := v.Deprecation; d != nil {
		m.Deprecation = &DeprecationBlock{AnnouncedDate: d.AnnouncedDate, EOLDate: d.EOLDate, ReplacedBy: d.ReplacedBy,
			MigrationGuide: d.MigrationGuide, MigrationScript: d.MigrationScript}
	}
	if g := v.GraphQL; g != nil {
		m.GraphQL = &PluginGraphQLBlock{Enabled: g.Enabled, SubgraphName: g.SubgraphName, SubgraphURL: g.SubgraphURL, SchemaPath: g.SchemaPath}
		for _, e := range g.Entities {
			m.GraphQL.Entities = append(m.GraphQL.Entities, PluginGraphQLEntityKey(e))
		}
	}
	return m
}

// permissionsFromV2 maps the v2 permissions value (array or object) onto the
// runtime PermissionSet. Validation already proved the shape.
func permissionsFromV2(p any) PermissionSet {
	switch v := p.(type) {
	case []any:
		out := PermissionSet{Canonical: make([]string, 0, len(v))}
		for _, e := range v {
			s, _ := e.(string)
			out.Canonical = append(out.Canonical, s)
		}
		return out
	case map[string]any:
		out := PermissionSet{Grouped: make(map[string][]string, len(v))}
		for k, e := range v {
			list, _ := e.([]any)
			for _, s := range list {
				str, _ := s.(string)
				out.Grouped[k] = append(out.Grouped[k], str)
			}
			if _, ok := out.Grouped[k]; !ok {
				out.Grouped[k] = []string{}
			}
		}
		return out
	}
	return PermissionSet{}
}

// licenseText is what plugin info shows as the licence: the v1 licence text
// carried in license_spdx when the file has one, else free or licensed.
func licenseText(v *manifestv2.Manifest) string {
	if v.LicenseSPDX != "" {
		return v.LicenseSPDX
	}
	return v.License
}
