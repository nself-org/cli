package main

// released_cli_test.go: a registry.json written by this tool must decode with
// the registry types of the released CLI (v1.4.12, internal/plugin/registry_parse.go),
// copied here unchanged apart from the three nested blocks the generated
// entries never carry (compat), whose shape does not matter.

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

type registryEnvelope struct {
	Version     string `json:"version"`
	LastUpdated string `json:"lastUpdated"`
	GeneratedAt string `json:"generated_at"`
	FetchedAt   string `json:"fetchedAt"`
	Tier        string `json:"tier"`
	// Plugins is raw so the object and array forms can be told apart.
	Plugins json.RawMessage `json:"plugins"`
}

type pluginImplementation struct {
	Language       string `json:"language"`
	Runtime        string `json:"runtime"`
	DefaultPort    int    `json:"defaultPort"`
	EntryPoint     string `json:"entryPoint"`
	CLI            string `json:"cli"`
	PackageManager string `json:"packageManager"`
	Framework      string `json:"framework"`
	PluginType     string `json:"pluginType"`
	BinaryName     string `json:"binaryName"`
}

type pluginChecksumsEntry struct {
	SHA256    string            `json:"sha256,omitempty"`
	Platforms map[string]string `json:"platforms,omitempty"`
}

type cliCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type deprecationBlock struct {
	AnnouncedDate   string `json:"announcedDate"`
	EOLDate         string `json:"eolDate"`
	ReplacedBy      string `json:"replacedBy,omitempty"`
	MigrationGuide  string `json:"migrationGuide"`
	MigrationScript string `json:"migrationScript,omitempty"`
}

type multiApp struct {
	Supported       bool   `json:"supported"`
	IsolationColumn string `json:"isolationColumn,omitempty"`
	PKStrategy      string `json:"pkStrategy,omitempty"`
	DefaultValue    string `json:"defaultValue,omitempty"`
}

type pluginGraphQLBlock struct {
	Enabled      bool   `json:"enabled"`
	SubgraphName string `json:"subgraph_name"`
	SubgraphURL  string `json:"subgraph_url"`
	SchemaPath   string `json:"schema_path,omitempty"`
}

type pluginEntry struct {
	Name                 string                `json:"name"`
	Version              string                `json:"version"`
	Description          string                `json:"description"`
	Category             string                `json:"category"`
	Tier                 string                `json:"tier"`
	License              string                `json:"license"`
	LicenseType          string                `json:"licenseType"`
	Repository           string                `json:"repository"`
	Checksum             string                `json:"checksum"`
	Checksums            *pluginChecksumsEntry `json:"checksums,omitempty"`
	DownloadURL          string                `json:"download_url"`
	RequiresLicense      bool                  `json:"requires_license"`
	Tags                 []string              `json:"tags"`
	Tables               []string              `json:"tables,omitempty"`
	Port                 int                   `json:"port,omitempty"`
	TierPair             bool                  `json:"tier_pair,omitempty"`
	Bundles              []string              `json:"bundles,omitempty"`
	Dependencies         json.RawMessage       `json:"dependencies,omitempty"`
	PublishStatus        string                `json:"status,omitempty"`
	AuthorPublicKey      string                `json:"author_public_key,omitempty"`
	Signature            string                `json:"signature,omitempty"`
	Implementation       *pluginImplementation `json:"implementation,omitempty"`
	Language             string                `json:"language,omitempty"`
	Runtime              string                `json:"runtime,omitempty"`
	PluginType           string                `json:"pluginType,omitempty"`
	BinaryName           string                `json:"binaryName,omitempty"`
	CLICommands          []cliCommand          `json:"cliCommands,omitempty"`
	Author               string                `json:"author,omitempty"`
	Homepage             string                `json:"homepage,omitempty"`
	IsCommercial         bool                  `json:"isCommercial,omitempty"`
	RequiredEntitlements []string              `json:"requiredEntitlements,omitempty"`
	MinNselfVersion      string                `json:"minNselfVersion,omitempty"`
	MaxNselfVersion      string                `json:"maxNselfVersion,omitempty"`
	MinNodeVersion       string                `json:"minNodeVersion,omitempty"`
	ArchSupport          []string              `json:"arch_support,omitempty"`
	EntryPoint           string                `json:"entryPoint,omitempty"`
	CLI                  string                `json:"cli,omitempty"`
	HealthEndpoint       string                `json:"health_endpoint,omitempty"`
	PackageManager       string                `json:"packageManager,omitempty"`
	Framework            string                `json:"framework,omitempty"`
	Views                []string              `json:"views,omitempty"`
	Deprecation          *deprecationBlock     `json:"deprecation,omitempty"`
	GraphQL              *pluginGraphQLBlock   `json:"graphql,omitempty"`
	APIEndpoints         json.RawMessage       `json:"apiEndpoints,omitempty"`
	UpdatedAt            string                `json:"updated_at,omitempty"`
}

// TestReleasedCLIDecodesGeneratedRegistries decodes both tiers the way
// parseRegistryJSON does (object form) and spot-checks the fields installs use.
func TestReleasedCLIDecodesGeneratedRegistries(t *testing.T) {
	free, lic := genBoth(t, t.TempDir())
	for _, c := range []struct{ dir, tier string }{{free, "free"}, {lic, "pro"}} {
		var env registryEnvelope
		if err := json.Unmarshal(readFile(t, filepath.Join(c.dir, "registry.json")), &env); err != nil {
			t.Fatalf("%s envelope: %v", c.tier, err)
		}
		if env.Tier != c.tier {
			t.Errorf("envelope tier = %q, want %q", env.Tier, c.tier)
		}
		var entries map[string]pluginEntry
		if err := json.Unmarshal(env.Plugins, &entries); err != nil {
			t.Fatalf("%s entries: %v", c.tier, err)
		}
		if len(entries) == 0 {
			t.Fatalf("%s: no entries decoded", c.tier)
		}
		for slug, e := range entries {
			if e.Name != slug || e.Version == "" || len(e.Checksum) != 64 || e.Tier != c.tier || e.Category == "" {
				t.Errorf("%s/%s: decoded entry lost a field the installer reads: %+v", c.tier, slug, e)
			}
		}
	}
	var env registryEnvelope
	_ = json.Unmarshal(readFile(t, filepath.Join(free, "registry.json")), &env)
	var entries map[string]pluginEntry
	_ = json.Unmarshal(env.Plugins, &entries)
	ai := entries["ai-cli"]
	if ai.Implementation == nil || ai.Implementation.BinaryName != "nself-ai" || ai.Implementation.PluginType != "cli" {
		t.Errorf("ai-cli: binaryName/pluginType must survive in implementation (install-to-proxy bridge): %+v", ai.Implementation)
	}
	if len(entries["sentry-cli"].CLICommands) < 2 || !entries["cron"].TierPair || len(entries["search"].Bundles) != 1 {
		t.Errorf("multi-command, tier_pair or bundles lost: %+v", entries["sentry-cli"])
	}
}
