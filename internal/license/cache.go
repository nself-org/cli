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
	"strconv"
	"time"

	"github.com/nself-org/cli/internal/compat"
)

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
	noteCacheWritten(entry)
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
	return c.AgeAt(time.Now())
}

// PublicKeyEntry holds a versioned Ed25519 public key. ID is the numeric kid
// (0 when the kid is not a number), KID the kid as ping writes it.
type PublicKeyEntry struct {
	ID  int
	KID string
	Key ed25519.PublicKey
}

// usableKey reports whether k is a 32-byte Ed25519 key that is not all zero.
func usableKey(k ed25519.PublicKey) bool {
	if len(k) != ed25519.PublicKeySize {
		return false
	}
	for _, b := range k {
		if b != 0 {
			return true
		}
	}
	return false
}

// GetPublicKeys returns the keys that may verify ping's signatures: the
// committed PingKeys plus extraKeys(), which is empty in every build except
// one tagged nself_devkeys (keys_release.go, keys_devkeys.go). A nil or
// all-zero key is dropped, never trusted.
func GetPublicKeys() []PublicKeyEntry {
	var out []PublicKeyEntry
	for _, k := range PingKeys {
		if usableKey(k.Public) {
			id, _ := strconv.Atoi(k.ID)
			out = append(out, PublicKeyEntry{ID: id, KID: k.ID, Key: k.Public})
		}
	}
	return append(out, extraKeys()...)
}

// IsZeroPubKey reports that no usable verification key exists. Callers fail
// closed on it (nothing verifies); it never means "skip verification".
func IsZeroPubKey() bool { return len(GetPublicKeys()) == 0 }

// GetEmbeddedPubKeyHex returns the hex of the first committed ping key, or an
// empty string when there is none.
func GetEmbeddedPubKeyHex() string {
	if ks := GetPublicKeys(); len(ks) > 0 {
		return hex.EncodeToString(ks[0].Key)
	}
	return ""
}

// clockMarkPath is the file holding the highest trusted time seen, next to the cache.
func clockMarkPath() (string, bool) {
	p, err := CachePath()
	if err != nil {
		return "", false
	}
	return filepath.Join(filepath.Dir(p), "license.clock"), true
}

// readTrustedTime returns the persisted highest trusted time (zero when none).
func readTrustedTime() time.Time {
	p, ok := clockMarkPath()
	if !ok {
		return time.Time{}
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return time.Time{}
	}
	n, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil || n <= 0 {
		return time.Time{}
	}
	return time.Unix(n, 0)
}

// writeTrustedTime persists t as the trusted-time mark. Best effort: it only
// writes where the cache directory already exists, and a failure changes nothing.
func writeTrustedTime(t time.Time) {
	p, ok := clockMarkPath()
	if !ok {
		return
	}
	if st, err := os.Stat(filepath.Dir(p)); err != nil || !st.IsDir() {
		return
	}
	_ = os.WriteFile(p, []byte(strconv.FormatInt(t.Unix(), 10)), 0o600)
}

// noteCacheWritten resets the trusted-time mark to the signed iat of a cache
// entry that was just written from a verified reply. A reply is only accepted
// inside its own 24 h window, so this is the recovery path after a clock that
// ran fast: going online puts the mark back to real time.
func noteCacheWritten(entry *CacheEntry) {
	// compat.V15(P7-PLUG-63): no trusted-time mark -> mark reset to the signed iat of each verified reply
	if !compat.V15() {
		return
	}
	if iat, ok := entry.signedIssuedAt(); ok {
		writeTrustedTime(time.Unix(iat, 0))
	}
}

// clockTrusted reports whether now can be believed for deciding on this entry:
// not behind the entry's signed iat by more than the skew, and not behind the
// highest time earlier checks trusted by more than clockTolerance. From v1.5 an
// entry read while the clock is behind that evidence is treated as untrusted
// (a clock rolled back to revive an old licence).
func (c *CacheEntry) clockTrusted(now time.Time) bool {
	// compat.V15(P7-PLUG-63): cache trusted whatever the clock says -> refused when the clock is behind the signed iat or the highest time seen
	if !compat.V15() {
		return true
	}
	if iat, ok := c.signedIssuedAt(); ok && iat > now.Unix()+replySkewSec {
		return false
	}
	hw := readTrustedTime()
	// Tolerance 10 min: NTP steps, DST bugs and sleep drift are not a rollback.
	return hw.IsZero() || !now.Before(hw.Add(-10*time.Minute))
}

// noteTrustedTime raises the mark to now (never lowers it) after a trusted decision.
func noteTrustedTime(now time.Time) {
	// compat.V15(P7-PLUG-63): no trusted-time mark -> mark raised to now after each trusted decision
	if compat.V15() && now.After(readTrustedTime()) {
		writeTrustedTime(now)
	}
}

// cacheWithinTerm reports whether the cached licence's signed expiry plus the
// post-expiry grace still covers now. The air-gap fail-open path is unbounded
// by cache age but never by the licence term.
func cacheWithinTerm(entry *CacheEntry, now time.Time) bool {
	// compat.V15(P7-PLUG-63): fail-open past the licence expiry -> refused once the signed expiry plus the post-expiry grace has passed
	if compat.V15() && entry.ExpiresAt > 0 && now.After(time.Unix(entry.ExpiresAt, 0).Add(PostExpiryGraceWindow)) {
		return false
	}
	return true
}

// graceStateFor is DetermineGraceState with the offline ceiling also applied
// after expiry: before expiry a cache older than GraceHardThreshold is
// read-only, and from v1.5 so is one inside the post-expiry grace.
func graceStateFor(entry *CacheEntry) GraceCheckResult {
	g := DetermineGraceState(entry)
	// compat.V15(P7-PLUG-63): post-expiry grace ignores cache age -> post-expiry write access also needs a cache younger than the 7-day offline ceiling
	if compat.V15() && g.State == GracePostExpiry && g.CacheAge >= GraceHardThreshold {
		g.WriteAllowed = false
		g.Message += " The cached validation is older than the 7-day offline ceiling: read-only until you connect."
	}
	return g
}
