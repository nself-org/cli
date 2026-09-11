// Package license provides shared license helpers for pro plugins: grace
// period tracking, offline validation cache, skip-verify for development.
//
// The authoritative validator runs server-side at ping.nself.org. This package
// only handles the consumer side: caching a last-good validation on disk,
// deciding when a stale cache is still acceptable, and short-circuiting in
// dev.
package license

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultCachePath returns the standard on-disk location for the license
// validation cache. Mirrors the path the CLI writes to via `nself license set`
// so plugin consumers and the CLI see the same cache.
func DefaultCachePath() string {
	if p := os.Getenv("NSELF_LICENSE_CACHE"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".nself", "license", "cache.json")
	}
	return filepath.Join(home, ".nself", "license", "cache.json")
}

// RemainingGrace reports how much grace period is left on a cached validation
// relative to now. Returns zero if the cache is expired, absent, or stale.
func (c CachedValidation) RemainingGrace(now time.Time, grace time.Duration) time.Duration {
	if c.ValidatedAt.IsZero() {
		return 0
	}
	if !c.ExpiresAt.IsZero() && now.After(c.ExpiresAt) {
		return 0
	}
	elapsed := now.Sub(c.ValidatedAt)
	if elapsed > grace {
		return 0
	}
	return grace - elapsed
}

// DefaultGracePeriod is how long a previously-valid key remains acceptable
// after ping.nself.org becomes unreachable. Seven days mirrors the CLI's
// offline-grace hard ceiling (GraceHardThreshold in
// internal/license/grace.go). It is a plain duplicate, not an import: this
// SDK is published as its own Go module (sdk/go/v2) and cannot depend on the
// CLI's internal/ packages, so the value is kept in sync by hand. If the
// CLI's ceiling changes, update this constant in the same change.
const DefaultGracePeriod = 7 * 24 * time.Hour

// CachedValidation is the on-disk record of the last successful validation.
type CachedValidation struct {
	KeyHash     string    `json:"key_hash"`
	Tier        string    `json:"tier"`
	Plugins     []string  `json:"plugins"`
	ValidatedAt time.Time `json:"validated_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// Fresh reports whether the cached validation is still trusted given a grace
// period. Cache is fresh when now <= ValidatedAt + grace.
func (c CachedValidation) Fresh(now time.Time, grace time.Duration) bool {
	if c.ValidatedAt.IsZero() {
		return false
	}
	if !c.ExpiresAt.IsZero() && now.After(c.ExpiresAt) {
		return false
	}
	return now.Sub(c.ValidatedAt) <= grace
}

// HashKey returns the sha256 of a license key. Stored instead of the raw key
// so the cache file is safe even if copied between machines.
func HashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

// Validator holds config for license checks.
type Validator struct {
	CachePath   string        // e.g. ~/.nself/license/cache.json
	GracePeriod time.Duration // default DefaultGracePeriod
	SkipVerify  bool          // set from NSELF_LICENSE_SKIP_VERIFY=1
}

// NewValidator applies defaults.
func NewValidator(cachePath string) *Validator {
	return &Validator{
		CachePath:   cachePath,
		GracePeriod: DefaultGracePeriod,
		SkipVerify:  strings.EqualFold(os.Getenv("NSELF_LICENSE_SKIP_VERIFY"), "1"),
	}
}

// Load reads the cache file. Returns an empty CachedValidation (no error) if
// the file does not exist.
func (v *Validator) Load() (CachedValidation, error) {
	var c CachedValidation
	data, err := os.ReadFile(v.CachePath)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("license: read cache: %w", err)
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("license: parse cache: %w", err)
	}
	return c, nil
}

// Save writes the cache to disk with 0600 perms (P15 lesson).
func (v *Validator) Save(c CachedValidation) error {
	if err := os.MkdirAll(filepath.Dir(v.CachePath), 0o700); err != nil {
		return fmt.Errorf("license: mkdir cache dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("license: marshal cache: %w", err)
	}
	if err := os.WriteFile(v.CachePath, data, 0o600); err != nil {
		return fmt.Errorf("license: write cache: %w", err)
	}
	return nil
}

// AllowPlugin returns nil if the plugin is permitted given the cached record.
// The cache must be fresh (within grace period) and must list the plugin name.
// SkipVerify=true (dev mode) always returns nil.
func (v *Validator) AllowPlugin(plugin, key string, now time.Time) error {
	if v.SkipVerify {
		return nil
	}
	cache, err := v.Load()
	if err != nil {
		return err
	}
	if cache.KeyHash == "" {
		return fmt.Errorf("license: no cached validation — run `nself license set <key>` and try again online")
	}
	if key != "" && cache.KeyHash != HashKey(key) {
		return fmt.Errorf("license: cached key does not match current key")
	}
	if !cache.Fresh(now, v.GracePeriod) {
		return fmt.Errorf("license: cached validation stale (last %s, grace %s) — reconnect to ping.nself.org to refresh",
			cache.ValidatedAt.Format(time.RFC3339), v.GracePeriod)
	}
	for _, p := range cache.Plugins {
		if p == plugin {
			return nil
		}
	}
	return fmt.Errorf("license: tier %q does not include plugin %q", cache.Tier, plugin)
}
