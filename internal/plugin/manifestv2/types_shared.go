package manifestv2

// Shared holds the v1 keys that v2 keeps with the same name and JSON type.
// The authoritative list is tools/manifestv2migrate/testdata/v1412-readers.txt
// (the keys every v1.4.12 reader decodes); TestReaderScan keeps it covered.
// Polymorphic v1 shapes (grouped dependencies, object-form webhooks and so on)
// are authored in their one canonical shape here; the v1 normalizer converts.
type Shared struct {
	Author     string   `json:"author,omitempty"`
	Homepage   string   `json:"homepage,omitempty"`
	Repository string   `json:"repository,omitempty"`
	Tags       []string `json:"tags,omitempty"`

	Language       string   `json:"language,omitempty"`
	Port           int      `json:"port,omitempty"`
	HealthEndpoint string   `json:"health_endpoint,omitempty"`
	PackageManager string   `json:"packageManager,omitempty"`
	Framework      string   `json:"framework,omitempty"`
	MinNodeVersion string   `json:"minNodeVersion,omitempty"`
	ArchSupport    []string `json:"arch_support,omitempty"`

	Tables []string `json:"tables,omitempty"`
	Views  []string `json:"views,omitempty"`

	APIEndpoints []string    `json:"apiEndpoints,omitempty"`
	Webhooks     []string    `json:"webhooks,omitempty"`
	RestRoutes   []RestRoute `json:"rest_routes,omitempty"`
	EnvVars      []EnvVar    `json:"envVars,omitempty"`

	Dependencies         []string            `json:"dependencies,omitempty"`
	OptionalDependencies []string            `json:"optionalDependencies,omitempty"`
	SystemDependencies   *SystemDependencies `json:"systemDependencies,omitempty"`
	Consumes             []string            `json:"consumes,omitempty"`
	Provides             []string            `json:"provides,omitempty"`

	MultiApp *MultiApp `json:"multiApp,omitempty"`
	// Permissions keeps its v1 shape: an array of canonical permission strings
	// or an object grouping actions by category. Validation checks the shape;
	// the fail-closed allowlist stays with the runtime (internal/plugin).
	Permissions any `json:"permissions,omitempty"`

	CompatBlock *CompatBlock  `json:"compat,omitempty"`
	Deprecation *Deprecation  `json:"deprecation,omitempty"`
	GraphQL     *GraphQLBlock `json:"graphql,omitempty"`

	RequiredEntitlements []string          `json:"requiredEntitlements,omitempty"`
	MaxNselfVersion      string            `json:"maxNselfVersion,omitempty"`
	Visibility           string            `json:"visibility,omitempty"`
	UpdatedAt            string            `json:"updated_at,omitempty"`
	PlatformChecksums    map[string]string `json:"platform_checksums,omitempty"`
}

// Compat holds the generated compatibility keys (Epic D1). They exist so
// released 1.4.x CLIs keep decoding a v2 file; authors never write them.
// Projection derives them; CheckCompat (E112) proves they match. They are
// removed at v1.6.0.
//
// EntryPoint, Runtime, CLI and the cliCommands entries other than the
// canonical command are carried verbatim from v1: no v2 field defines them.
// Tier and LicenseType are carried for a licensed plugin (max, cloud, internal)
// and generated as free for a free one.
type Compat struct {
	PluginType      string       `json:"pluginType,omitempty"`
	BinaryName      string       `json:"binaryName,omitempty"`
	CLICommands     []CLICommand `json:"cliCommands,omitempty"`
	EntryPoint      string       `json:"entryPoint,omitempty"`
	Runtime         string       `json:"runtime,omitempty"`
	CLI             string       `json:"cli,omitempty"`
	MinNselfVersion string       `json:"minNselfVersion,omitempty"`
	Status          string       `json:"status,omitempty"`
	IsCommercial    bool         `json:"isCommercial,omitempty"`
	LicenseType     string       `json:"licenseType,omitempty"`
	RequiresLicense bool         `json:"requires_license,omitempty"`
	Tier            string       `json:"tier,omitempty"`
}

// CLICommand is one v1 cliCommands entry.
type CLICommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// EnvVar describes an environment variable (v1 shape).
type EnvVar struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
	Default     string `json:"default,omitempty"`
}

// SystemDependency is a host-level dependency (v1 shape).
type SystemDependency struct {
	Name          string `json:"name"`
	Verify        string `json:"verify"`
	MinVersion    string `json:"minVersion,omitempty"`
	Apt           string `json:"apt,omitempty"`
	Brew          string `json:"brew,omitempty"`
	CustomInstall string `json:"custom_install,omitempty"`
}

// SystemDependencies groups required and recommended host dependencies.
type SystemDependencies struct {
	Required    []SystemDependency `json:"required,omitempty"`
	Recommended []SystemDependency `json:"recommended,omitempty"`
}

// Deprecation is the v1 deprecation block plus the v2 state.
type Deprecation struct {
	AnnouncedDate   string `json:"announcedDate,omitempty"`
	EOLDate         string `json:"eolDate,omitempty"`
	ReplacedBy      string `json:"replacedBy,omitempty"`
	MigrationGuide  string `json:"migrationGuide,omitempty"`
	MigrationScript string `json:"migrationScript,omitempty"`
	// State is deprecated or eol; released CLIs ignore it.
	State string `json:"state,omitempty"`
}

// MultiApp describes multi-tenancy (v1 shape).
type MultiApp struct {
	Supported       bool   `json:"supported"`
	IsolationColumn string `json:"isolationColumn,omitempty"`
	PKStrategy      string `json:"pkStrategy,omitempty"`
	DefaultValue    string `json:"defaultValue,omitempty"`
}

// CompatBlock is the v1 `compat` key: CLI and service version ranges.
type CompatBlock struct {
	Nself    string            `json:"nself,omitempty"`
	Requires map[string]string `json:"requires,omitempty"`
}

// GraphQLEntityKey names an Apollo Federation entity type.
type GraphQLEntityKey struct {
	Type string `json:"type"`
	Key  string `json:"key"`
}

// GraphQLBlock registers the plugin as a federation subgraph (v1 shape).
type GraphQLBlock struct {
	Enabled      bool               `json:"enabled"`
	SubgraphName string             `json:"subgraph_name"`
	SubgraphURL  string             `json:"subgraph_url"`
	SchemaPath   string             `json:"schema_path,omitempty"`
	Entities     []GraphQLEntityKey `json:"entities,omitempty"`
}

// RestRoute is one entry of the v1 rest_routes key (read by apidocs). Auth and
// HMAC are optional additions that carry the v1 routes entries' auth scheme and
// the name of the env var holding the webhook secret; released readers ignore them.
type RestRoute struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	Summary string `json:"summary,omitempty"`
	Auth    string `json:"auth,omitempty"`
	HMAC    string `json:"hmac,omitempty"`
}
