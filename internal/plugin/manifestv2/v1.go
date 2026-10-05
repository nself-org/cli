package manifestv2

import (
	"encoding/json"
	"strings"
)

// v1Manifest decodes a v1 plugin.json permissively, like released CLIs do:
// every key a v1.4.12 reader decodes, nothing else. Polymorphic keys stay raw
// until v1shapes.go converts them.
type v1Manifest struct {
	Name, Version, Description, Category, License string
	Author, Homepage, Repository                  string
	Tags                                          []string
	IsCommercial                                  bool     `json:"isCommercial"`
	LicenseType                                   string   `json:"licenseType"`
	RequiredEntitlements                          []string `json:"requiredEntitlements"`
	RequiresLicense                               bool     `json:"requires_license"`
	MinNselfVersion                               string   `json:"minNselfVersion"`
	MinNodeVersion                                string   `json:"minNodeVersion"`
	ArchSupport                                   []string `json:"arch_support"`
	Language, Runtime                             string
	Port                                          int
	EntryPoint                                    string `json:"entryPoint"`
	CLI                                           string `json:"cli"`
	PluginType                                    string `json:"pluginType"`
	BinaryName                                    string `json:"binaryName"`
	HealthEndpoint                                string `json:"health_endpoint"`
	PackageManager                                string `json:"packageManager"`
	Framework                                     string
	Tables, Views                                 []string
	APIEndpoints                                  json.RawMessage `json:"apiEndpoints"`
	Webhooks                                      json.RawMessage
	CLICommands                                   []CLICommand    `json:"cliCommands"`
	EnvVars                                       json.RawMessage `json:"envVars"`
	Dependencies                                  json.RawMessage
	OptionalDependencies                          []string        `json:"optionalDependencies"`
	SystemDependencies                            json.RawMessage `json:"systemDependencies"`
	Consumes, Provides                            []string
	MultiApp                                      *MultiApp `json:"multiApp"`
	Permissions                                   any
	CompatBlock                                   *CompatBlock `json:"compat"`
	Tier                                          string
	Status                                        string
	Deprecation                                   *Deprecation
	GraphQL                                       *GraphQLBlock     `json:"graphql"`
	MaxNselfVersion                               string            `json:"maxNselfVersion"`
	UpdatedAt                                     string            `json:"updated_at"`
	PlatformChecksums                             map[string]string `json:"platform_checksums"`
	Visibility                                    string
	Installable                                   *bool       `json:"installable"`
	RestRoutes                                    []RestRoute `json:"rest_routes"`
}

// Normalize converts v1 plugin.json bytes into a validated v2 Manifest. It is
// the one v1 reader: the shape rules of released CLIs (grouped dependencies,
// object-form webhooks and envVars, object-form apiEndpoints), the status and
// licence tables of Epic D1, the commands mapping (binaryName becomes
// commands.binary, the cliCommands entry named like it becomes commands.command,
// `binaryName: null` becomes `commands: null`) and the service block. It checks
// what v1.4.12's validateManifest checks and no more (validateV1), so a file
// the released CLI accepts always normalizes; Validate is for v2-native input.
// Keys no v1.4.12 reader decodes are not mapped: UnmappedV1Keys lists them and
// the migrate tool refuses to write a file that has any.
func Normalize(data []byte) (*Manifest, error) {
	var v v1Manifest
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, invalid("plugin.json", err.Error())
	}
	maturity, state, err := fromV1Status(v.Status)
	if err != nil {
		return nil, err
	}
	m := &Manifest{ManifestVersion: ManifestVersion, Name: v.Name, Version: v.Version, Description: v.Description,
		Category: v.Category, Maturity: maturity, Installable: v.Installable}
	m.License, m.LicenseSPDX = LicenseFree, v.License
	if v.IsCommercial || v.RequiresLicense || (v.LicenseType != "" && v.LicenseType != "free") || (v.Tier != "" && v.Tier != "free") {
		m.License = LicenseLicensed
	}
	if err := fillShared(m, &v); err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(data, &raw)
	x := mapExtras(raw)
	m.Capabilities, m.Env = x.Capabilities, x.Env
	if len(m.RestRoutes) == 0 {
		m.RestRoutes = x.RestRoutes
	}
	if state != "" {
		if m.Deprecation == nil {
			m.Deprecation = &Deprecation{}
		}
		m.Deprecation.State = state
	}
	if v.MinNselfVersion != "" {
		m.Requires = &Requires{Nself: ">=" + v.MinNselfVersion}
	}
	if len(v.Tables) > 0 {
		s := "np_" + strings.ReplaceAll(v.Name, "-", "_")
		m.Schema = &s
	}
	m.Commands = v1Commands(&v)
	m.Service = v1Service(&v, m.Commands != nil)
	m.EntryPoint, m.Runtime, m.CLI, m.CLICommands = v.EntryPoint, v.Runtime, v.CLI, v.CLICommands
	m.Tier, m.LicenseType = v.Tier, v.LicenseType
	ApplyProjection(m)
	Canonicalize(m)
	if err := validateV1(m); err != nil {
		return nil, err
	}
	return m, nil
}

// fromV1Status is the v1 status to maturity table (Epic D1). alpha is not in
// the v1 enum but 15 shipped manifests use it; absent means stable.
func fromV1Status(s string) (maturity, state string, err error) {
	switch s {
	case "", "stable", "beta":
		return MaturityImplemented, "", nil
	case "experimental", "alpha":
		return MaturityExperimental, "", nil
	case "planned":
		return MaturityPlanned, "", nil
	case "deprecated":
		return MaturityDeferred, StateDeprecated, nil
	case "eol":
		return MaturityDeferred, StateEOL, nil
	}
	return "", "", invalidf("status", "unknown plugin status %q", s)
}

// v1Commands maps binaryName and cliCommands to the commands block, or nil for
// a plugin that publishes no binary.
func v1Commands(v *v1Manifest) *Commands {
	if v.PluginType != "" && v.PluginType != "cli" {
		return nil
	}
	bin := v.BinaryName
	if bin == "" && v.PluginType == "cli" {
		bin = "nself-" + v.Name
	}
	if bin == "" {
		return nil
	}
	c := &Commands{Command: v.Name, Binary: bin}
	want := strings.TrimPrefix(bin, "nself-")
	for _, e := range v.CLICommands {
		if e.Name == want {
			c.Command, c.Summary = want, e.Description
			break
		}
	}
	return c
}

// v1Service derives the service block a v1 file does not state: a CLI binary
// without a port is kind cli, a port means compose, otherwise library.
func v1Service(v *v1Manifest, hasCommands bool) *Service {
	s := &Service{Kind: KindLibrary}
	switch {
	case v.Port > 0:
		s.Kind = KindCompose
		compose, hc := "docker-compose.plugin.yml", v.HealthEndpoint
		if hc == "" {
			hc = "/health"
		}
		port := v.Port
		s.Compose, s.Healthcheck, s.Port = &compose, &hc, &port
	case hasCommands:
		s.Kind = KindCLI
	}
	return s
}
