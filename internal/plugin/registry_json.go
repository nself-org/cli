package plugin

// registry_json.go — full registry JSON parsing and manifest conversion.
//
// Purpose: parse a full registry JSON document into pluginEntry values and convert an entry into the manifest shape the rest of the plugin package expects, split out of registry.go for file size.
// Inputs: raw JSON bytes fetched by Fetch/fetchFromURL in registry.go.
// Outputs: parsed pluginEntry values and the manifest built from entryToManifest, plus the last-fetched timestamp via RegistryFetchedAt.
// Constraints: pure move from registry.go (CLI-R12 Batch F); no behaviour change.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func parseRegistryJSON(data []byte) (*Registry, error) {
	var env registryEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("parse registry JSON: %w", err)
	}

	if len(env.Plugins) == 0 {
		return &Registry{FetchedAt: env.FetchedAt}, nil
	}

	// Detect format by the first non-whitespace byte.
	trimmed := strings.TrimSpace(string(env.Plugins))
	if len(trimmed) == 0 || trimmed == "null" {
		return &Registry{FetchedAt: env.FetchedAt}, nil
	}

	var plugins []PluginManifest

	switch trimmed[0] {
	case '[':
		// Array format (pro registry).
		var entries []pluginEntry
		if err := json.Unmarshal(env.Plugins, &entries); err != nil {
			return nil, fmt.Errorf("parse registry array: %w", err)
		}
		plugins = make([]PluginManifest, 0, len(entries))
		for _, e := range entries {
			plugins = append(plugins, entryToManifest(e))
		}

	case '{':
		// Object format (free registry): {"plugin_name": {...}, ...}
		var entries map[string]pluginEntry
		if err := json.Unmarshal(env.Plugins, &entries); err != nil {
			return nil, fmt.Errorf("parse registry object: %w", err)
		}
		plugins = make([]PluginManifest, 0, len(entries))
		for key, e := range entries {
			// In object format the key is the plugin name. The inner
			// object may or may not repeat the name field.
			if e.Name == "" {
				e.Name = key
			}
			plugins = append(plugins, entryToManifest(e))
		}

	default:
		return nil, fmt.Errorf("unexpected registry plugins format (starts with %q)", string(trimmed[0]))
	}

	return &Registry{Plugins: plugins, FetchedAt: env.FetchedAt}, nil
}

// RegistryFetchedAt returns the top-level "fetchedAt" snapshot timestamp from
// the most recently cached registry response, or "" when no cache exists or
// the cached registry didn't carry one (CLI-R16). It reads the on-disk cache
// only — no network call — so it is safe to call right after a List/Fetch
// that already populated the cache in the same command invocation.
func RegistryFetchedAt() string {
	data, err := os.ReadFile(filepath.Join(defaultCacheDir(), registryCacheFile))
	if err != nil {
		return ""
	}
	reg, err := parseRegistryJSON(data)
	if err != nil {
		return ""
	}
	return reg.FetchedAt
}

func entryToManifest(e pluginEntry) PluginManifest {
	tier := e.Tier
	if tier == "" {
		// Derive tier from license info when not explicit.
		if e.LicenseType == "pro" || e.RequiresLicense {
			tier = "pro"
		} else {
			tier = "free"
		}
	}

	// Resolve port: prefer flat Port field, fall back to implementation.defaultPort.
	port := e.Port
	if port == 0 && e.Implementation != nil {
		port = e.Implementation.DefaultPort
	}

	// Resolve implementation fields from the nested block when flat fields are absent.
	language := e.Language
	runtime := e.Runtime
	pluginType := e.PluginType
	binaryName := e.BinaryName
	var implEntryPoint, implCLI, implPackageManager, implFramework string
	if e.Implementation != nil {
		if language == "" {
			language = e.Implementation.Language
		}
		if runtime == "" {
			runtime = e.Implementation.Runtime
		}
		// A flat field wins when present; otherwise take the nested one. Real
		// registry entries put these under implementation, and dropping them
		// here is what made every CLI plugin install into a dead command.
		if pluginType == "" {
			pluginType = e.Implementation.PluginType
		}
		if binaryName == "" {
			binaryName = e.Implementation.BinaryName
		}
		implEntryPoint = e.Implementation.EntryPoint
		implCLI = e.Implementation.CLI
		implPackageManager = e.Implementation.PackageManager
		implFramework = e.Implementation.Framework
	}

	var platformChecksums map[string]string
	if e.Checksums != nil {
		platformChecksums = normalizePlatformChecksums(e.Checksums.Platforms)
	}

	return PluginManifest{
		Name:              e.Name,
		Version:           e.Version,
		Description:       e.Description,
		Category:          e.Category,
		License:           e.License,
		LicenseType:       e.LicenseType,
		Tier:              tier,
		Repository:        e.Repository,
		Checksum:          e.Checksum,
		PlatformChecksums: platformChecksums,
		Tags:              e.Tags,
		RequiresLicense:   e.RequiresLicense,
		Tables:            e.Tables,
		Port:              port,
		TierPair:          e.TierPair,
		Bundles:           e.Bundles,
		Dependencies:      parseDependencies(e.Dependencies),
		APIEndpoints:      parseAPIEndpoints(e.APIEndpoints),
		Language:          language,
		Runtime:           runtime,
		PluginType:        pluginType,
		BinaryName:        binaryName,
		CLICommands:       e.CLICommands,

		Author:               e.Author,
		Homepage:             e.Homepage,
		IsCommercial:         e.IsCommercial,
		RequiredEntitlements: e.RequiredEntitlements,
		MinNselfVersion:      e.MinNselfVersion,
		MaxNselfVersion:      e.MaxNselfVersion,
		MinNodeVersion:       e.MinNodeVersion,
		ArchSupport:          e.ArchSupport,
		EntryPoint:           firstNonEmptyStr(e.EntryPoint, implEntryPoint),
		CLI:                  firstNonEmptyStr(e.CLI, implCLI),
		HealthEndpoint:       e.HealthEndpoint,
		PackageManager:       firstNonEmptyStr(e.PackageManager, implPackageManager),
		Framework:            firstNonEmptyStr(e.Framework, implFramework),
		Views:                e.Views,
		MultiApp:             e.MultiApp,
		Deprecation:          e.Deprecation,
		GraphQL:              e.GraphQL,
		Compat:               e.Compat,
		PublishStatus:        e.PublishStatus,
		AuthorPublicKey:      e.AuthorPublicKey,
		Signature:            e.Signature,
		UpdatedAt:            e.UpdatedAt,
	}
}

// firstNonEmptyStr returns the first non-empty argument, so a flat registry
// field wins over the same value nested under implementation.
func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
