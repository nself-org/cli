package plugin

// registry_cache.go — on-disk registry response cache.
//
// Purpose: read and write the cached registry response (including a stale/max-age variant for offline use) and marshal a registry client for logging, used by Fetch in registry.go, split out for file size.
// Inputs: the registry cache path and a fetched registryEnvelope.
// Outputs: cached JSON read from or written to disk.
// Constraints: pure move from registry.go (CLI-R12 Batch F); no behaviour change.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/cli/internal/security"
)

func (c *registryHTTPClient) cachePath() string {
	return filepath.Join(c.cacheDir, registryCacheFile)
}

// readCache returns the registry from cache only if the file exists
// and is younger than cacheTTL.
func (c *registryHTTPClient) readCache() (*Registry, error) {
	return c.readCacheWithMaxAge(c.cacheTTL)
}

// readStaleCache returns the registry from cache regardless of age.
func (c *registryHTTPClient) readStaleCache() (*Registry, error) {
	return c.readCacheWithMaxAge(0)
}

func (c *registryHTTPClient) readCacheWithMaxAge(maxAge time.Duration) (*Registry, error) {
	p := c.cachePath()
	info, err := os.Stat(p)
	if err != nil {
		return nil, err
	}

	// If maxAge > 0, enforce freshness.
	if maxAge > 0 && time.Since(info.ModTime()) > maxAge {
		return nil, fmt.Errorf("cache expired")
	}

	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}

	reg, err := parseRegistryJSON(data)
	if err != nil {
		return nil, err
	}
	var scope struct {
		Origin string `json:"origin"`
	}
	_ = json.Unmarshal(data, &scope)
	reg.Origin = scope.Origin
	if !c.originAccepted(reg.Origin, maxAge == 0) {
		return nil, fmt.Errorf("cache is from another registry")
	}
	return reg, nil
}

// originAccepted reports whether a cache recording origin may answer for this
// client (D-0849). An exclusive client accepts only its own registry. Others
// also accept a file with no recorded origin (older CLI, or the plan pin of
// `nself update --yes`) and, as the last resort, the fallback host's cache.
func (c *registryHTTPClient) originAccepted(origin string, lastResort bool) bool {
	if origin == c.primaryURL {
		return true
	}
	if c.exclusive {
		return false
	}
	return origin == "" || (lastResort && origin == c.fallbackURL)
}

func (c *registryHTTPClient) writeCache(reg *Registry) error {
	if c.cacheDir == "" {
		return nil
	}
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return err
	}

	// Re-serialize as a normalized JSON blob for the cache.
	data, err := json.Marshal(reg)
	if err != nil {
		return err
	}
	cachePath := c.cachePath()
	if err := os.WriteFile(cachePath, data, 0600); err != nil {
		return err
	}
	return security.EnforceFilePermissions(cachePath, 0600)
}

// MarshalJSON implements json.Marshaler for Registry so the cache
// round-trips through the array format consistently.
func (r Registry) MarshalJSON() ([]byte, error) {
	type envelope struct {
		Version   string        `json:"version"`
		FetchedAt string        `json:"fetchedAt,omitempty"`
		Origin    string        `json:"origin,omitempty"`
		Plugins   []pluginEntry `json:"plugins"`
	}
	entries := make([]pluginEntry, 0, len(r.Plugins))
	for _, p := range r.Plugins {
		// Re-serialise APIEndpoints ([]string) as a JSON string array so the
		// cache file is always in the normalised string format, which
		// parseAPIEndpoints can read back without ambiguity.
		var rawEPs json.RawMessage
		if len(p.APIEndpoints) > 0 {
			b, err := json.Marshal(p.APIEndpoints)
			if err != nil {
				return nil, fmt.Errorf("marshaling api endpoints for %q: %w", p.Name, err)
			}
			rawEPs = b
		}
		var rawDeps json.RawMessage
		if len(p.Dependencies) > 0 {
			b, err := json.Marshal(p.Dependencies)
			if err != nil {
				return nil, fmt.Errorf("marshaling dependencies for %q: %w", p.Name, err)
			}
			rawDeps = b
		}
		// Checksums re-serialises PlatformChecksums into the nested registry
		// shape, matching how it was parsed in — see pluginChecksumsEntry.
		// SHA256 is left empty: the cache's source-tarball checksum lives in
		// the flat Checksum field below (and always has), so re-populating
		// the nested duplicate here would just be inventing a value never
		// read back by anything (entryToManifest sources PlatformChecksums
		// from Checksums.Platforms only).
		var checksums *pluginChecksumsEntry
		if len(p.PlatformChecksums) > 0 {
			checksums = &pluginChecksumsEntry{Platforms: p.PlatformChecksums}
		}

		entries = append(entries, pluginEntry{
			Name:            p.Name,
			Version:         p.Version,
			Description:     p.Description,
			Category:        p.Category,
			License:         p.License,
			LicenseType:     p.LicenseType,
			RequiresLicense: p.RequiresLicense,
			Tier:            p.Tier,
			Repository:      p.Repository,
			Checksum:        p.Checksum,
			Checksums:       checksums,
			Tags:            p.Tags,
			Tables:          p.Tables,
			Port:            p.Port,
			TierPair:        p.TierPair,
			Bundles:         p.Bundles,
			Dependencies:    rawDeps,
			APIEndpoints:    rawEPs,
			Compat:          p.Compat,
			UpdatedAt:       p.UpdatedAt,
			Catalog:         catalogFor(p.PostgresExtensions),

			// Implementation fields. Their absence here is what made every
			// CLI plugin install into a dead command: the first request
			// parsed pluginType/binaryName correctly, this re-serialisation
			// dropped them, and the very next read came from the cache with
			// both empty — so linkCLIBinary saw a plugin that declared no
			// command and did nothing. Language and Runtime were being lost
			// the same way, which is the tell I should have followed sooner.
			Language:    p.Language,
			Runtime:     p.Runtime,
			PluginType:  p.PluginType,
			BinaryName:  p.BinaryName,
			CLICommands: p.CLICommands,

			// Signature material. Its absence here meant a warm cache handed
			// verifyPluginSignature three empty strings, which takes the
			// "unsigned" branch — and because publishStatus was empty too, that
			// branch returns nil instead of refusing a stable plugin. Signature
			// verification was being skipped on every cache-warm install.
			PublishStatus:    p.PublishStatus,
			AuthorPublicKey:  p.AuthorPublicKey,
			Signature:        p.Signature,
			ReleaseSignature: releaseSignatureRaw(p.ReleaseSignature),

			Author:               p.Author,
			Homepage:             p.Homepage,
			IsCommercial:         p.IsCommercial,
			RequiredEntitlements: p.RequiredEntitlements,
			MinNselfVersion:      p.MinNselfVersion,
			MaxNselfVersion:      p.MaxNselfVersion,
			MinNodeVersion:       p.MinNodeVersion,
			ArchSupport:          p.ArchSupport,
			EntryPoint:           p.EntryPoint,
			CLI:                  p.CLI,
			HealthEndpoint:       p.HealthEndpoint,
			PackageManager:       p.PackageManager,
			Framework:            p.Framework,
			Views:                p.Views,
			MultiApp:             p.MultiApp,
			Deprecation:          p.Deprecation,
			GraphQL:              p.GraphQL,
		})
	}
	return json.Marshal(envelope{
		Version:   "1.0.0",
		FetchedAt: r.FetchedAt,
		Origin:    r.Origin,
		Plugins:   entries,
	})
}
