package plugin

// license_hmac_entitlements.go — HMAC signing and entitlement caching.
//
// Purpose: sign/verify license payloads with HMAC, recognize the owner all-access key, and cache/check plugin entitlements, used by the license validation flow in license.go, split out for file size.
// Inputs: a license payload or HMAC key material, and the entitlements returned by a validation call.
// Outputs: a verified/signed payload, or a cached entitlement decision.
// Constraints: pure move from license.go (CLI-R12 Batch F); no behaviour change.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// hmacSign returns the HMAC-SHA256 of data using key (raw bytes), encoded as a
// hex string.
func hmacSign(data string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

// hmacVerify checks that expectedHMAC matches the HMAC-SHA256 of data using
// key (raw bytes). The comparison is timing-safe.
func hmacVerify(data string, expectedHMAC string, key []byte) bool {
	expected, err := hex.DecodeString(expectedHMAC)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return hmac.Equal(mac.Sum(nil), expected)
}

// loadHMACKeyOrFallback returns the persisted random HMAC key. On any error it
// falls back to a zero-length key, which causes all HMAC verifications to fail
// and forces a remote re-validation. This is always safer than exposing the
// observable-derived machineID as the HMAC key.
func loadHMACKeyOrFallback() []byte {
	key, err := loadHMACKey()
	if err != nil {
		// Return empty key — callers treat a failed HMAC as a cache miss and
		// re-validate remotely, which is the correct fail-safe behavior.
		return []byte{}
	}
	return key
}

// isOwnerKey returns true if the license key has the nself_owner_ prefix.
// Owner keys receive special treatment: no machine fingerprint and no cache
// expiry.
func isOwnerKey(key string) bool {
	return strings.HasPrefix(key, "nself_owner_")
}

// entitlementCache is the JSON structure persisted to entitlements.json.
type entitlementCache struct {
	Tier     string   `json:"tier"`
	Plugins  []string `json:"plugins"`
	CachedAt string   `json:"cached_at"`
}

// entitlementCacheTTL is how long a cached entitlements file is considered
// fresh. After this duration, CheckEntitlements reports a cache miss so the
// caller falls through to remote validation.
const entitlementCacheTTL = 24 * time.Hour

// CacheEntitlements writes the tier-to-plugins mapping to
// {cacheDir}/entitlements.json with 0600 permissions.
func cacheEntitlements(cacheDir string, tier string, plugins []string) error {
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return fmt.Errorf("creating entitlement cache directory: %w", err)
	}
	c := entitlementCache{
		Tier:     tier,
		Plugins:  plugins,
		CachedAt: time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshalling entitlement cache: %w", err)
	}
	return os.WriteFile(filepath.Join(cacheDir, "entitlements.json"), data, 0600)
}

// CheckEntitlements reads {cacheDir}/entitlements.json and checks whether
// pluginName is in the cached plugins list. It returns two booleans:
//
//   - allowed: true if pluginName appears in the cached list.
//   - found:   true if a valid, non-expired cache file was read.
//
// If the file is missing, unparsable, or older than 24 hours, found is false
// and the caller should fall through to remote validation.
func checkEntitlements(cacheDir string, pluginName string) (allowed bool, found bool) {
	data, err := os.ReadFile(filepath.Join(cacheDir, "entitlements.json"))
	if err != nil {
		return false, false
	}
	var c entitlementCache
	if err := json.Unmarshal(data, &c); err != nil {
		return false, false
	}
	cachedAt, err := time.Parse(time.RFC3339, c.CachedAt)
	if err != nil {
		return false, false
	}
	if time.Since(cachedAt) > entitlementCacheTTL {
		return false, false
	}
	for _, p := range c.Plugins {
		if p == pluginName {
			return true, true
		}
	}
	return false, true
}

// cachedEntitlementTier returns the tier stored in a fresh entitlements.json,
// or "" when the file is missing, unparsable or older than the TTL.
func cachedEntitlementTier(cacheDir string) string {
	data, err := os.ReadFile(filepath.Join(cacheDir, "entitlements.json"))
	if err != nil {
		return ""
	}
	var c entitlementCache
	if json.Unmarshal(data, &c) != nil {
		return ""
	}
	cachedAt, err := time.Parse(time.RFC3339, c.CachedAt)
	if err != nil || time.Since(cachedAt) > entitlementCacheTTL {
		return ""
	}
	return c.Tier
}

// LicenseValidateResponse is the JSON body returned by the license validation
// endpoint on HTTP 200. The Tier and Plugins fields are used to populate the
// local entitlement cache.
type LicenseValidateResponse struct {
	Valid   bool     `json:"valid"`
	Reason  string   `json:"reason,omitempty"`
	Tier    string   `json:"tier"`
	Plugins []string `json:"plugins"`
	Expires string   `json:"expires,omitempty"`
}

// licenseValidateResponse is an alias kept for internal use.
type licenseValidateResponse = LicenseValidateResponse

// ValidateLicenseRemoteWithDetails performs a remote license check and returns
// the full response including tier, plugins, and expiry information.
func ValidateLicenseRemoteWithDetails(ctx context.Context, key string, pingURL string) (bool, *LicenseValidateResponse, error) {
	return validateLicenseRemoteWithEntitlements(ctx, key, pingURL)
}

// keyPrefix extracts the prefix portion of a license key (everything up to and
// including the third underscore-delimited segment). For example,
// "nself_pro_abc123..." returns "nself_pro_".
func keyPrefix(key string) string {
	for _, prefix := range validLicensePrefixes {
		if strings.HasPrefix(key, prefix) {
			return prefix
		}
	}
	// Fallback: first 10 chars or the whole key if shorter.
	if len(key) > 10 {
		return key[:10]
	}
	return key
}
