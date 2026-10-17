package plugin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// DefaultRegistryURL is the primary registry endpoint (Cloudflare Worker).
	DefaultRegistryURL = "https://plugins.nself.org/registry.json"

	// FallbackRegistryURL is the GitHub raw fallback when the primary is down.
	FallbackRegistryURL = "https://raw.githubusercontent.com/nself-org/plugins/main/registry.json"

	// DefaultCacheTTL is the registry cache lifetime in seconds.
	DefaultCacheTTL = 300

	// registryCacheFile is the filename used inside the cache directory.
	registryCacheFile = "registry.json"

	// httpTimeout is the per-request timeout for registry fetches.
	httpTimeout = 15 * time.Second
)

// registryHTTPClient implements RegistryClient using HTTP with caching
// and a two-endpoint fallback chain.
type registryHTTPClient struct {
	primaryURL  string
	fallbackURL string
	cacheDir    string
	cacheTTL    time.Duration
	httpClient  *http.Client
	// exclusive makes Fetch ask only primaryURL: no fallback host, no stale
	// cache, and a cache is used only if it records primaryURL as its origin.
	exclusive bool
}

// NewRegistryClient creates a RegistryClient that fetches from the given
// primary URL with automatic fallback and local file caching.
// If registryURL is empty, NSELF_PLUGIN_REGISTRY is checked first, then
// DefaultRegistryURL is used as the final fallback.
func newRegistryClient(registryURL, cacheDir string, cacheTTLSeconds int) RegistryClient {
	if registryURL == "" {
		if envURL := os.Getenv("NSELF_PLUGIN_REGISTRY"); envURL != "" {
			registryURL = envURL
		} else {
			registryURL = DefaultRegistryURL
		}
	}
	// S-011: Reject non-HTTPS registry URLs except localhost for dev.
	registryURL = enforceRegistryHTTPS(registryURL)
	if cacheTTLSeconds <= 0 {
		cacheTTLSeconds = DefaultCacheTTL
	}
	return &registryHTTPClient{
		primaryURL:  registryURL,
		fallbackURL: FallbackRegistryURL,
		cacheDir:    cacheDir,
		cacheTTL:    time.Duration(cacheTTLSeconds) * time.Second,
		httpClient: &http.Client{
			Timeout: httpTimeout,
		},
	}
}

// Fetch retrieves the full plugin registry. It follows the fallback chain:
//  1. Try primaryURL
//  2. Try fallbackURL
//  3. Return stale cache (if any)
func (c *registryHTTPClient) Fetch(ctx context.Context) (*Registry, error) {
	// Check fresh cache first.
	if reg, err := c.readCache(); err == nil {
		return reg, nil
	}

	// Try primary endpoint.
	reg, err := c.fetchFromURL(ctx, c.primaryURL)
	if err == nil {
		reg.Origin = c.primaryURL
		_ = c.writeCache(reg)
		return reg, nil
	}
	primaryErr := err
	if c.exclusive {
		return nil, fmt.Errorf("registry fetch failed (%s only, no fallback): %w", c.primaryURL, primaryErr)
	}

	// Try fallback endpoint.
	reg, err = c.fetchFromURL(ctx, c.fallbackURL)
	if err == nil {
		reg.Origin = c.fallbackURL
		_ = c.writeCache(reg)
		return reg, nil
	}

	// Both endpoints failed. Try stale cache as last resort.
	if reg, cacheErr := c.readStaleCache(); cacheErr == nil {
		return reg, nil
	}

	return nil, fmt.Errorf("registry fetch failed (primary: %v, fallback: %v)", primaryErr, err)
}

// GetPlugin looks up a single plugin by name.
func (c *registryHTTPClient) GetPlugin(ctx context.Context, name string) (*PluginManifest, error) {
	reg, err := c.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	for i := range reg.Plugins {
		if strings.EqualFold(reg.Plugins[i].Name, name) {
			return &reg.Plugins[i], nil
		}
	}
	return nil, fmt.Errorf("plugin %q not found in registry", name)
}

// FetchRegistry is the standalone entry point for fetching a registry.
// It creates an ephemeral client and delegates to Fetch.
//
// Fallback chain:
//  1. registryURL (default: https://plugins.nself.org/registry.json)
//  2. GitHub raw fallback
//  3. Stale cache at cacheDir/registry.json
//
// A cached file answers only for the registry it was fetched from (or the
// fallback host, or a file with no recorded origin); see FetchRegistryExclusive
// for the mode that accepts nothing else.
func FetchRegistry(ctx context.Context, registryURL string, cacheDir string) (*Registry, error) {
	return newFetchRegistryClient(registryURL, cacheDir, false).Fetch(ctx)
}

// FetchRegistryExclusive fetches from the configured registry only (registryURL,
// else NSELF_PLUGIN_REGISTRY, else the default). It never uses the GitHub
// fallback or a stale cache, and it reads a cache file only if that file records
// the same registry as its origin. A failure is an error. The returned Registry
// carries Origin, so WriteRegistryCache pins it as scoped to that registry.
func FetchRegistryExclusive(ctx context.Context, registryURL string, cacheDir string) (*Registry, error) {
	return newFetchRegistryClient(registryURL, cacheDir, true).Fetch(ctx)
}

func newFetchRegistryClient(registryURL, cacheDir string, exclusive bool) RegistryClient {
	if registryURL == "" {
		if envURL := os.Getenv("NSELF_PLUGIN_REGISTRY"); envURL != "" {
			registryURL = envURL
		}
	}
	if registryURL != "" {
		registryURL = enforceRegistryHTTPS(registryURL)
	}
	client := newRegistryClient(registryURL, cacheDir, DefaultCacheTTL)
	client.(*registryHTTPClient).exclusive = exclusive
	return client
}

// ParseRegistry parses a registry response body (the object or the array
// plugin format) without touching the network, a cache or the fallback host. A
// body that does not parse is an error. Callers that must not fall back (the
// strict reads of `nself update`, P7-LIVE-10) use it instead of FetchRegistry.
func ParseRegistry(body []byte) (*Registry, error) {
	return parseRegistryJSON(body)
}

// WriteRegistryCache stores reg as the shared registry cache file
// (cacheDir/registry.json, mode 0600), in the same normalised form a normal
// fetch writes, so the next Install within the cache TTL resolves versions from
// reg. `nself update --yes` uses it to pin the plugin steps to the registry the
// plan was computed from.
func WriteRegistryCache(cacheDir string, reg *Registry) error {
	if cacheDir == "" {
		return fmt.Errorf("write registry cache: empty cache directory")
	}
	if reg == nil {
		return fmt.Errorf("write registry cache: nil registry")
	}
	return (&registryHTTPClient{cacheDir: cacheDir}).writeCache(reg)
}

// enforceRegistryHTTPS validates that a registry URL uses HTTPS.
// HTTP is allowed only for localhost/127.0.0.1 (dev use). Non-HTTPS
// URLs are replaced with the default secure registry URL.
func enforceRegistryHTTPS(rawURL string) string {
	if strings.HasPrefix(rawURL, "https://") {
		return rawURL
	}
	// Allow plain HTTP for local development.
	if strings.HasPrefix(rawURL, "http://localhost") || strings.HasPrefix(rawURL, "http://127.0.0.1") {
		return rawURL
	}
	fmt.Fprintf(os.Stderr, "warning: rejecting non-HTTPS registry URL %q, using default\n", rawURL)
	return DefaultRegistryURL
}

// --- HTTP fetch ---

func (c *registryHTTPClient) fetchFromURL(ctx context.Context, url string) (*Registry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "nself-cli")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP GET %s: status %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response from %s: %w", url, err)
	}

	return parseRegistryJSON(body)
}
