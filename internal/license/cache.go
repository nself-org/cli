// Package license — cache.go implements the local license cache with Ed25519
// signature verification and grace-period-aware freshness checks.
//
// Cache location: ~/.cache/nself/license.json (0600)
// Fields: key_hash, tier, plugins_allowed, fetched_at, expires_at, signature
package license

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// licensePubKeyHex is injected via -X ldflag at goreleaser build time.
// In dev builds without ldflags, it remains empty — license verification is
// disabled and nself version prints a warning banner.
//
//nolint:gochecknoglobals
var licensePubKeyHex = "" //nolint:unused // set via -X github.com/nself-org/cli/internal/license.licensePubKeyHex=<hex>

// IsZeroPubKey reports whether the build was made without an ldflags-injected
// signing key. Returns true when licensePubKeyHex is empty OR consists entirely
// of '0' characters (e.g., a placeholder 64-char zero string).
// goreleaser injects a real non-zero Ed25519 pubkey hex; dev builds leave it empty.
//
// Exception: when LICENSE_PUBLIC_KEY_OVERRIDE is set to a valid non-zero Ed25519
// public key hex, IsZeroPubKey returns false so that tests can exercise the
// production signature-verification code path without goreleaser ldflags.
func IsZeroPubKey() bool {
	// Check override first — allows tests to exercise the sig-verify path.
	if override := os.Getenv("LICENSE_PUBLIC_KEY_OVERRIDE"); override != "" {
		keyBytes, err := hex.DecodeString(override)
		if err == nil && len(keyBytes) == ed25519.PublicKeySize {
			// Non-zero override key supplied: treat as "real key embedded".
			for _, b := range keyBytes {
				if b != 0 {
					return false
				}
			}
		}
	}
	if licensePubKeyHex == "" {
		return true
	}
	for _, ch := range licensePubKeyHex {
		if ch != '0' {
			return false
		}
	}
	return true
}

// CacheEntry represents a cached license validation response with Ed25519
// signature from the server.
type CacheEntry struct {
	KeyHash        string   `json:"key_hash"`
	Tier           string   `json:"tier"`
	PluginsAllowed []string `json:"plugins_allowed"`
	FetchedAt      int64    `json:"fetched_at"`
	ExpiresAt      int64    `json:"expires_at"`
	Signature      string   `json:"signature"`
	SignatureKeyID int      `json:"signature_key_id"`
}

// defaultCacheDir returns ~/.cache/nself.
func defaultCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determining home directory: %w", err)
	}
	return filepath.Join(home, ".cache", "nself"), nil
}

// CachePath returns the full path to the license cache file.
// It respects LICENSE_CACHE_PATH if set.
func CachePath() (string, error) {
	if p := os.Getenv("LICENSE_CACHE_PATH"); p != "" {
		return p, nil
	}
	dir, err := defaultCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "license.json"), nil
}

// ReadCache reads and parses the license cache file.
// Returns nil, nil if the cache file does not exist.
func ReadCache() (*CacheEntry, error) {
	path, err := CachePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// KEEP: cold == absent. checker.go's grace paths collapse them
			// anyway (`err != nil || entry == nil`), so this fails CLOSED.
			return nil, nil
		}
		return nil, fmt.Errorf("reading license cache: %w", err)
	}
	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("parsing license cache: %w", err)
	}
	return &entry, nil
}

// WriteCache writes the cache entry to disk with 0600 permissions using an
// atomic tmpfile + rename pattern so partial writes never corrupt an existing
// cache. (D3-T10: prevent torn writes that would otherwise force fail-closed.)
func WriteCache(entry *CacheEntry) error {
	path, err := CachePath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := ensureDir(dir); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling cache entry: %w", err)
	}

	// Atomic write: write to a sibling tmpfile, then rename. Rename is atomic
	// on POSIX filesystems for paths within the same directory.
	tmp, err := os.CreateTemp(dir, ".license.json.tmp.*")
	if err != nil {
		return fmt.Errorf("creating temp cache file: %w", err)
	}
	tmpPath := tmp.Name()
	// Best-effort cleanup if anything below fails before rename.
	cleanup := func() { _ = os.Remove(tmpPath) }

	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("chmod temp cache file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("writing temp cache file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("syncing temp cache file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("closing temp cache file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("renaming temp cache file: %w", err)
	}
	return nil
}

// DeleteCache removes the license cache file.
func DeleteCache() error {
	path, err := CachePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing license cache: %w", err)
	}
	return nil
}

// HashKey returns the SHA-256 hex digest of a license key.
func HashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// CacheAge returns how long ago the cache was fetched.
func (c *CacheEntry) CacheAge() time.Duration {
	return time.Since(time.Unix(c.FetchedAt, 0))
}

// VerifySignature verifies the cache entry's Ed25519 signature against the
// bundled public keys. It accepts the current key (keyID N) and the previous
// key (keyID N-1) to support rotation windows.
func (c *CacheEntry) VerifySignature() bool {
	keys := GetPublicKeys()
	for _, pk := range keys {
		if c.SignatureKeyID != 0 && pk.ID != c.SignatureKeyID {
			continue
		}
		sigBytes, err := hex.DecodeString(c.Signature)
		if err != nil {
			continue
		}
		// The signed payload is the JSON of the entry without the signature fields.
		payload := c.signablePayload()
		if ed25519.Verify(pk.Key, payload, sigBytes) {
			return true
		}
	}
	// If keyID was specified and didn't match, try all keys (rotation window).
	if c.SignatureKeyID != 0 {
		sigBytes, err := hex.DecodeString(c.Signature)
		if err != nil {
			return false
		}
		payload := c.signablePayload()
		for _, pk := range keys {
			if ed25519.Verify(pk.Key, payload, sigBytes) {
				return true
			}
		}
	}
	return false
}

// signablePayload produces the deterministic byte sequence that was signed by
// the server. This must match the server's signing format exactly.
//
// Canonical format: key_hash|tier|fetched_at|expires_at|plugins_allowed_sorted_joined
//
// PluginsAllowed is sorted alphabetically and joined with commas before being
// included in the payload. This prevents an attacker with home-directory write
// access from injecting arbitrary plugin names into the cached JSON while the
// Ed25519 signature still passes (SIEGE V03-F01).
func (c *CacheEntry) signablePayload() []byte {
	sorted := make([]string, len(c.PluginsAllowed))
	copy(sorted, c.PluginsAllowed)
	sort.Strings(sorted)
	pluginsField := strings.Join(sorted, ",")
	return []byte(fmt.Sprintf("%s|%s|%d|%d|%s",
		c.KeyHash, c.Tier, c.FetchedAt, c.ExpiresAt, pluginsField))
}

// PublicKeyEntry holds a versioned Ed25519 public key.
type PublicKeyEntry struct {
	ID  int
	Key ed25519.PublicKey
}

// bundledPublicKeys contains the Ed25519 public keys used to verify license
// cache signatures. Key ID 1 is the current key. During rotation, both N and
// N-1 are accepted.
//
// The public key override env var LICENSE_PUBLIC_KEY_OVERRIDE can replace key 1
// for testing.
var bundledPublicKeys []PublicKeyEntry

func init() {
	// D3-T01: load the Ed25519 public key injected at goreleaser build time via
	// -X github.com/nself-org/cli/internal/license.licensePubKeyHex=<hex>.
	// Dev builds leave licensePubKeyHex empty → zero key → IsZeroPubKey() returns true
	// → license signature verification is skipped (CLI falls back to bare validation).
	if licensePubKeyHex != "" && !IsZeroPubKey() {
		if keyBytes, err := hex.DecodeString(licensePubKeyHex); err == nil &&
			len(keyBytes) == ed25519.PublicKeySize {
			bundledPublicKeys = []PublicKeyEntry{
				{ID: 1, Key: ed25519.PublicKey(keyBytes)},
			}
			return
		}
	}
	// Fallback: zero key (dev builds without ldflags). Signature verification
	// will always return false for this key, which IsZeroPubKey() signals to callers.
	devKey := make(ed25519.PublicKey, ed25519.PublicKeySize)
	bundledPublicKeys = []PublicKeyEntry{
		{ID: 1, Key: devKey},
	}
}

// GetPublicKeys returns the active public keys, respecting the
// LICENSE_PUBLIC_KEY_OVERRIDE environment variable for testing.
func GetPublicKeys() []PublicKeyEntry {
	if override := os.Getenv("LICENSE_PUBLIC_KEY_OVERRIDE"); override != "" {
		keyBytes, err := hex.DecodeString(override)
		if err == nil && len(keyBytes) == ed25519.PublicKeySize {
			return []PublicKeyEntry{{ID: 1, Key: ed25519.PublicKey(keyBytes)}}
		}
	}
	return bundledPublicKeys
}

// GetEmbeddedPubKeyHex returns the hex-encoded Ed25519 public key that was
// injected at build time via goreleaser ldflags (NSELF_LICENSE_PUBKEY_HEX).
// Returns an empty string in dev builds without ldflags.
// D3-T01: used by `nself license pubkey` and pubkey-refresh flow (D3-T10).
func GetEmbeddedPubKeyHex() string {
	return licensePubKeyHex
}
