package model

import "encoding/json"

// Registry is registry.json: the released-CLI registry v2 shape plus the
// `_generated` and `generated_from` keys. Plugins is keyed by slug.
type Registry struct {
	Generated         string                   `json:"_generated"`
	GeneratedFrom     GeneratedFrom            `json:"generated_from"`
	SchemaVersion     string                   `json:"schema_version"`
	Tier              string                   `json:"tier"`
	ChecksumAlgorithm string                   `json:"checksum_algorithm"`
	PluginsCount      int                      `json:"plugins_count"`
	Plugins           map[string]RegistryEntry `json:"plugins"`
}

// Implementation is the nested block released CLIs read for pluginType and
// binaryName (registry_json.go:107-127 at v1.4.12).
type Implementation struct {
	Language   string `json:"language,omitempty"`
	Runtime    string `json:"runtime,omitempty"`
	EntryPoint string `json:"entryPoint,omitempty"`
	PluginType string `json:"pluginType,omitempty"`
	BinaryName string `json:"binaryName,omitempty"`
}

// CLICommand is one cliCommands entry.
type CLICommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Checksums is the nested checksum object of a free entry.
type Checksums struct {
	SHA256    string            `json:"sha256"`
	Platforms map[string]string `json:"platforms,omitempty"`
}

// RegistryEntry is one plugin in registry.json. Key names and types are the
// basis entries' (released CLIs decode them); ReleaseSignature and Catalog are
// additive keys that v1.4.12 ignores.
type RegistryEntry struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	Description     string `json:"description"`
	Category        string `json:"category"`
	Tier            string `json:"tier"`
	License         string `json:"license"`
	MinNselfVersion string `json:"minNselfVersion,omitempty"`
	// MinNselfVersionSnake duplicates MinNselfVersion under its older key.
	MinNselfVersionSnake string          `json:"min_nself_version,omitempty"`
	RequiresLicense      bool            `json:"requires_license"`
	Language             string          `json:"language,omitempty"`
	Tags                 []string        `json:"tags"`
	Tables               []string        `json:"tables"`
	Author               string          `json:"author,omitempty"`
	Port                 int             `json:"port,omitempty"`
	Installable          *bool           `json:"installable,omitempty"`
	Dependencies         json.RawMessage `json:"dependencies"`
	Implementation       *Implementation `json:"implementation,omitempty"`
	CLICommands          []CLICommand    `json:"cliCommands,omitempty"`
	TierPair             bool            `json:"tier_pair,omitempty"`
	Bundles              []string        `json:"bundles,omitempty"`

	Checksum         string     `json:"checksum"`
	Checksums        *Checksums `json:"checksums,omitempty"`
	Tarball          string     `json:"tarball,omitempty"`
	TarballURL       string     `json:"tarballUrl,omitempty"`
	DownloadURL      string     `json:"download_url,omitempty"`
	ReleaseTag       string     `json:"releaseTag,omitempty"`
	ReleaseSignature *Signature `json:"release_signature,omitempty"`
	Catalog          *Info      `json:"catalog,omitempty"`
}
