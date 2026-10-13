package bundle

// installer_helpers_licence.go — the plugin-level licence pre-flight (Phase 2)
// of `nself bundle install` (D-0372).
//
// Purpose: refuse before any filesystem change when a member needs a licence
// the operator does not have. Each member is resolved the way plugin.Install
// resolves it (plugin.ResolvePlugin), so a free/licensed tier pair such as cron
// is judged by the entry that will actually be installed, never by whichever
// registry entry with that name came last.
// Inputs: the planned member names, the registry, the configured licence keys
// and the cached entitlements (<licence cache dir>/entitlements.json).
// Outputs: nil, or an error naming the first member that is refused.
// Constraints: read-only use of internal/plugin. A registry that cannot be
// fetched defers to plugin.Install, which checks every member again. The
// cached plugin list is consulted only for a free bundle (a paid bundle was
// just granted as a unit by Phase 1, and a stale cache must not refuse it) and
// only with exactly one key configured (the cache holds one key's answer). A
// cached all-access tier (license.IsAllAccessTier) allows every plugin: its
// cached list is empty by design. A cached MISS is not final: like plugin.Install
// the pre-flight confirms with the licence server first (licencePreflight.confirm),
// so a key upgraded within the 24 h cache TTL is not refused. The key is only
// handed to plugin.ValidateLicenseRemoteWithDetails; it is never printed,
// logged or put in an error.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/plugin"
)

// entitlementCacheFile and entitlementCacheTTL mirror the cache that
// internal/plugin writes after a remote licence validation
// (license_hmac_entitlements.go); a file older than the TTL is not trusted.
const (
	entitlementCacheFile = "entitlements.json"
	entitlementCacheTTL  = 24 * time.Hour
)

// errNoLicenceKey is the "no key at all" refusal: one error, not one per member.
var errNoLicenceKey = errors.New("bundle requires at least one paid plugin; no license key configured. Run 'nself license set <key>' or visit nself.org/pricing")

// cachedEntitlements is the part of entitlements.json the pre-flight reads.
type cachedEntitlements struct {
	Tier     string   `json:"tier"`
	Plugins  []string `json:"plugins"`
	CachedAt string   `json:"cached_at"`
}

// allows reports whether the cached answer includes plugin name.
func (c cachedEntitlements) allows(name string) bool {
	if license.IsAllAccessTier(c.Tier) {
		return true
	}
	for _, p := range c.Plugins {
		if strings.EqualFold(strings.TrimSpace(p), name) {
			return true
		}
	}
	return false
}

// readCachedEntitlements reads dir/entitlements.json. ok is false when the file
// is missing, unparsable or older than entitlementCacheTTL at now.
func readCachedEntitlements(dir string, now time.Time) (c cachedEntitlements, ok bool) {
	data, err := os.ReadFile(filepath.Join(dir, entitlementCacheFile))
	if err != nil || json.Unmarshal(data, &c) != nil {
		return cachedEntitlements{}, false
	}
	at, err := time.Parse(time.RFC3339, c.CachedAt)
	if err != nil || now.Sub(at) > entitlementCacheTTL {
		return cachedEntitlements{}, false
	}
	return c, true
}

// licencePreflight holds what the check needs, so tests inject every input.
type licencePreflight struct {
	tier     string                 // tier override for a tier pair, as plugin.Install reads it
	entitled plugin.EntitlementFunc // nil: plugin.ResolvePlugin's default (ping_api)
	keys     int                    // licence keys configured
	cache    *cachedEntitlements    // nil: no usable cached answer, or not consulted
	cacheDir string                 // named in the refusal
	// confirm re-checks a cached miss with the licence server (nil: the cached
	// miss refuses as before). allowed false with a nil error means the server
	// confirmed the key's tier does not include the plugin.
	confirm func(ctx context.Context, name string) (allowed bool, err error)
}

// check resolves every planned member and refuses the first licensed one the
// operator cannot install.
func (pf licencePreflight) check(ctx context.Context, reg *plugin.Registry, plugins []string) error {
	for _, name := range plugins {
		m, err := plugin.ResolvePlugin(ctx, reg, name, pf.tier, pf.entitled)
		if errors.Is(err, errs.ErrPluginNotFound) {
			continue // plugin.Install reports the proper "not found"
		}
		if err != nil {
			return fmt.Errorf("plugin %q: %w", name, err)
		}
		if !m.RequiresLicense {
			continue
		}
		if pf.keys == 0 {
			return errNoLicenceKey
		}
		if pf.cache != nil && !pf.cache.allows(m.Name) {
			if err := pf.refuseCachedMiss(ctx, m.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

// refuseCachedMiss decides a licensed member the cached entitlements omit: nil
// when the licence server says the key does include it, else the refusal.
func (pf licencePreflight) refuseCachedMiss(ctx context.Context, name string) error {
	cacheFile := filepath.Join(pf.cacheDir, entitlementCacheFile)
	cached := fmt.Errorf("plugin %q is licensed and the cached entitlements of your key (tier %q) do not include it. Visit nself.org/pricing, or remove %s to re-check with the licence server",
		name, pf.cache.Tier, cacheFile)
	if pf.confirm == nil {
		return cached
	}
	allowed, err := pf.confirm(ctx, name)
	switch {
	case err != nil:
		return fmt.Errorf("%w; the licence server could not be reached to re-check (%v)", cached, err)
	case !allowed:
		return fmt.Errorf("plugin %q is licensed and the licence server confirmed that your key's tier does not include it (last cached tier %q). Visit nself.org/pricing to upgrade",
			name, pf.cache.Tier)
	}
	return nil
}

// licenceKeys lists the configured licence keys, deduplicated: the canonical
// collector plus the owner all-access key, a vault convenience it does not
// know about. Callers must never print the values.
func licenceKeys() []string {
	keys := license.CollectLicenseKeys()
	if owner := os.Getenv("NSELF_PLUGIN_LICENSE_KEY_OWNER"); owner != "" {
		dup := false
		for _, k := range keys {
			dup = dup || k == owner
		}
		if !dup {
			keys = append(keys, owner)
		}
	}
	return keys
}

// licenceKeyCount counts the configured licence keys.
func licenceKeyCount() int { return len(licenceKeys()) }

// hasAnyLicenseKey returns true if the operator has any license key set (env,
// the stored key at ~/.nself/license/key, or the owner key). Never re-derive the
// key locations here: a hand-rolled lookup once read a legacy path and refused
// every paying customer.
func hasAnyLicenseKey() bool { return licenceKeyCount() > 0 }

// licenceChecker returns the production Phase 2 check for a bundle; freeBundle
// turns on the cached-entitlement test (see the file header).
func licenceChecker(freeBundle bool) func(ctx context.Context, plugins []string) error {
	return func(ctx context.Context, plugins []string) error {
		if len(plugins) == 0 {
			return nil
		}
		reg, err := plugin.FetchRegistry(ctx, "", defaultRegistryCacheDir())
		if err != nil {
			return nil // network failure: plugin.Install's offline logic decides
		}
		keys := licenceKeys()
		pf := licencePreflight{tier: os.Getenv("NSELF_PLUGIN_INSTALL_TIER"), keys: len(keys), cacheDir: plugin.LicenseCacheDir()}
		if freeBundle && pf.keys == 1 {
			if c, ok := readCachedEntitlements(pf.cacheDir, time.Now()); ok {
				pf.cache = &c
				pf.confirm = remoteConfirmer(keys[0])
			}
		}
		return pf.check(ctx, reg, plugins)
	}
}

// defaultLicenseChecker is the Phase 2 check without the cached-entitlement
// test (the paid-bundle form).
func defaultLicenseChecker(ctx context.Context, plugins []string) error {
	return licenceChecker(false)(ctx, plugins)
}

// validateRemote is the licence-server call, a variable so tests need no network.
var validateRemote = plugin.ValidateLicenseRemoteWithDetails

// remoteConfirmer returns the production licencePreflight.confirm for the single
// configured key. The server URL follows internal/plugin/paths.go pingAPIURL:
// NSELF_PING_API_URL without a trailing slash, else https://ping.nself.org.
// allowed is a valid key whose tier is all-access or whose plugin list names
// the plugin (case-insensitive). The key is never printed or put in an error.
func remoteConfirmer(key string) func(ctx context.Context, name string) (bool, error) {
	return func(ctx context.Context, name string) (bool, error) {
		pingURL := "https://ping.nself.org"
		if u := os.Getenv("NSELF_PING_API_URL"); u != "" {
			pingURL = strings.TrimRight(u, "/")
		}
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		valid, resp, err := validateRemote(ctx, key, pingURL)
		if err != nil {
			return false, err
		}
		if !valid || resp == nil {
			return false, nil
		}
		if license.IsAllAccessTier(resp.Tier) {
			return true, nil
		}
		for _, p := range resp.Plugins {
			if strings.EqualFold(strings.TrimSpace(p), name) {
				return true, nil
			}
		}
		return false, nil
	}
}
