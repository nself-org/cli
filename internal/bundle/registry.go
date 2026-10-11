// registry.go — fetch+cache loader for bundles.json (ADR-P6-03).
//
// Load is called once, eagerly, at command start (bundleCmd's
// PersistentPreRunE) — never lazily on first Get/All access. It fetches the
// live document from plugins.nself.org/bundles.json, validates it, and
// populates the in-memory bundle map every invocation. On fetch failure it
// falls back to the last-known-good local cache (mirrors
// internal/license/validate.go's RefreshCache/ExportCache/ImportCache
// cache-then-fall-back convention); if no cache exists either, Load returns
// a clear network error rather than silently serving an empty bundle set.
//
// There is no go:embed fallback — the loader always fetches live, cache
// fallback only on fetch failure (per ticket P6-E4-W3-S3-T10).
package bundle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/cli/internal/plugin"
)

const (
	// DefaultBundlesURL is the canonical bundles.json endpoint (T8's CF
	// worker). Override with NSELF_BUNDLES_URL for staging/tests.
	DefaultBundlesURL = "https://plugins.nself.org/bundles.json"

	bundlesHTTPTimeout   = 15 * time.Second
	bundlesCacheFileName = "bundles.json"

	// bundlesStaleWarnAfter is the age beyond which a served-from-cache
	// bundle set gets a one-time stderr warning (mirrors the license
	// validator's warn-on-stale pattern in defaultWarnOnce).
	bundlesStaleWarnAfter = 24 * time.Hour
)

// jsonBundle mirrors one entry of bundles.json's "bundles" map
// (bundles/bundles-schema.json in the plugins-pro repo).
type jsonBundle struct {
	Display      string   `json:"display"`
	Tier         string   `json:"tier"` // "free" | "paid"
	PriceMonthly float64  `json:"price_monthly"`
	PriceYearly  float64  `json:"price_yearly"`
	Plugins      []string `json:"plugins"`
}

// bundlesDoc is the top-level bundles.json shape.
type bundlesDoc struct {
	SchemaVersion string                `json:"schema_version"`
	Bundles       map[string]jsonBundle `json:"bundles"`
}

// cachedBundlesDoc wraps bundlesDoc with the timestamp of the fetch that
// produced it, so a served-from-cache response can report its own staleness.
type cachedBundlesDoc struct {
	FetchedAt int64      `json:"fetched_at"`
	Doc       bundlesDoc `json:"doc"`
}

var (
	stateMu       sync.RWMutex
	state         map[string]Bundle // nil until Load/LoadBytes succeeds
	staleWarnOnce sync.Once
)

// bundlesURL returns the configured bundles.json source, honoring
// NSELF_BUNDLES_URL for staging/offline-mirror overrides.
func bundlesURL() string {
	if u := os.Getenv("NSELF_BUNDLES_URL"); u != "" {
		return u
	}
	return DefaultBundlesURL
}

// Load eager-fetches and validates bundles.json, populating the resolver
// used by Get/Names/All/IsInstallable. Call once at command start. On fetch
// failure, falls back to the local cache (with a staleness warning past
// bundlesStaleWarnAfter); with no fetch and no cache, returns an error
// naming the network issue rather than leaving the resolver empty.
func Load(ctx context.Context) error {
	data, fetchErr := fetchBundlesJSON(ctx, bundlesURL())
	if fetchErr == nil {
		if err := LoadBytes(data); err != nil {
			return fmt.Errorf("validating bundles.json: %w", err)
		}
		_ = writeBundlesCache(data)
		return nil
	}

	cached, cacheErr := readBundlesCache()
	if cacheErr != nil || cached == nil {
		return fmt.Errorf("fetching bundles.json from %s: %w (no local cache available)", bundlesURL(), fetchErr)
	}
	age := time.Since(time.Unix(cached.FetchedAt, 0))
	if age > bundlesStaleWarnAfter {
		staleWarnOnce.Do(func() {
			fmt.Fprintf(os.Stderr,
				"warning: bundles.json cache is %s stale (last fetch failed: %v); bundle membership may be outdated. Reconnect and retry.\n",
				age.Round(time.Hour), fetchErr)
		})
	}
	if err := validateCatalogNames(cached.Doc); err != nil {
		return fmt.Errorf("cached bundles.json: %w", err)
	}
	setState(buildBundleMap(cached.Doc))
	return nil
}

// EnsureLoaded populates the resolver if it has not been loaded yet in this
// process — a no-op once Load/LoadBytes has already succeeded once. This is
// the safety net for callers OUTSIDE the `nself bundle` command family
// (which eagerly calls Load in its PersistentPreRunE): `nself build`,
// `start`, `restart`, `stop`, `ops`, and `status` all reach bundle expansion
// via internal/build's manifest loader without ever running through that
// PreRunE, so without this, a manifest's `bundles:` entries would silently
// expand to nothing in every one of those commands (P6-E4-W3-S3-T10 CI
// regression: TestLoadProjectManifest_FlatPluginsAndBundleExpansion).
// Deliberately NOT a refresh-on-every-call: once state is non-nil, this
// returns immediately, matching Load's own "eager once, not lazy-per-Get"
// design intent, just anchored at the first real caller in a process
// instead of assuming that caller is always the bundle subcommand tree.
func EnsureLoaded(ctx context.Context) error {
	stateMu.RLock()
	loaded := state != nil
	stateMu.RUnlock()
	if loaded {
		return nil
	}
	return Load(ctx)
}

// LoadBytes parses and validates a raw bundles.json payload and installs it
// as the current bundle set. Exported so tests (and any future offline
// import path) can seed a fixture document without a network round trip.
func LoadBytes(data []byte) error {
	var doc bundlesDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("invalid bundles.json: %w", err)
	}
	if !strings.HasPrefix(doc.SchemaVersion, "2.") {
		return fmt.Errorf("unsupported bundles.json schema_version %q (expected 2.x)", doc.SchemaVersion)
	}
	if len(doc.Bundles) == 0 {
		return fmt.Errorf("bundles.json has no bundles")
	}
	if err := validateCatalogNames(doc); err != nil {
		return err
	}
	setState(buildBundleMap(doc))
	return nil
}

// validateCatalogNames refuses a catalog whose member plugin names are not plain
// plugin slugs. Member names are joined to the plugin directory when a bundle is
// planned, installed or removed, so a path-shaped name must never get that far.
func validateCatalogNames(doc bundlesDoc) error {
	for slug, jb := range doc.Bundles {
		for _, name := range jb.Plugins {
			if err := plugin.ValidateName(name); err != nil {
				return fmt.Errorf("bundles.json bundle %q lists an invalid plugin: %w", slug, err)
			}
		}
	}
	return nil
}

// currentBundles returns the loaded bundle map, or an empty map if Load has
// not run yet (Get/Names/All degrade to "unknown bundle" rather than panic).
func currentBundles() map[string]Bundle {
	stateMu.RLock()
	defer stateMu.RUnlock()
	return state
}

func setState(m map[string]Bundle) {
	stateMu.Lock()
	state = m
	stateMu.Unlock()
}

// buildBundleMap converts the raw JSON document into the resolver's Bundle
// map, computing the ɳSelf+ meta-bundle as the union of every tier=="paid"
// bundle's plugins — never a hand-listed set (ADR-P6-03).
func buildBundleMap(doc bundlesDoc) map[string]Bundle {
	out := make(map[string]Bundle, len(doc.Bundles)+1)
	unionSet := make(map[string]struct{})
	for slug, jb := range doc.Bundles {
		out[slug] = Bundle{
			Name:    jb.Display,
			Slug:    slug,
			Price:   formatPrice(jb.Tier, jb.PriceMonthly, jb.PriceYearly),
			Plugins: jb.Plugins,
		}
		if jb.Tier == "paid" {
			for _, p := range jb.Plugins {
				unionSet[p] = struct{}{}
			}
		}
	}
	union := make([]string, 0, len(unionSet))
	for p := range unionSet {
		union = append(union, p)
	}
	sort.Strings(union)
	out[selfPlusSlug] = Bundle{
		Name:        "ɳSelf+",
		Slug:        selfPlusSlug,
		Price:       "$3.99/mo or $39.99/yr",
		Description: "All paid bundles + all apps + support",
		Plugins:     union,
	}
	return out
}

func formatPrice(tier string, monthly, yearly float64) string {
	if tier == "free" || (monthly == 0 && yearly == 0) {
		return "FREE"
	}
	return fmt.Sprintf("$%.2f/mo or $%.2f/yr", monthly, yearly)
}

// fetchBundlesJSON performs the HTTP GET against url and returns the raw
// response body (capped at 1MiB — bundles.json is a few KB).
func fetchBundlesJSON(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building bundles.json request: %w", err)
	}
	client := &http.Client{Timeout: bundlesHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}
