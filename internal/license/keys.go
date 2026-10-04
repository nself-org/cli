// Package license — multi-key support for product-based licensing.
//
// Keys are collected from environment variables:
//   - NSELF_PLUGIN_LICENSE_KEY (legacy single key)
//   - NSELF_LICENSE_KEY_1 through NSELF_LICENSE_KEY_10
//
// And from the stored key file at ~/.nself/license/key.
package license

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// PingKey is a ping_api Ed25519 public key; ID is the kid in the licence JWT
// header. The same key signs X-NSelf-License-Sig.
type PingKey struct {
	ID     string
	Public ed25519.PublicKey
}

// PingKeys is the committed set of ping public keys (public data: the pair
// behind NSELF_LICENSE_PUBKEY_HEX). A rotation adds the new kid before ping
// signs with it. A key that fails to decode is nil and never verifies.
var PingKeys = []PingKey{
	{ID: "1", Public: pubKeyFromHex("0ac4c2d7ec30d23bf2be55775e7170bd24a7544bad036d0d19f2a4deb516e00c")},
}

// ProductPrefix maps key prefixes to product display names.
type ProductPrefix struct {
	Prefix      string
	Product     string
	DisplayName string
}

// validProductPrefixes lists all accepted license key prefixes with their
// product names. Order matters: more specific prefixes must come first.
var validProductPrefixes = []ProductPrefix{
	{"nself_owner_", "owner", "ɳSelf Owner"},
	{"nself_plus_", "plus", "ɳSelf+"},
	{"nself_claw_", "claw", "ɳClaw"},
	{"nself_clawde_", "clawde", "ClawDE"},
	{"nself_chat_", "chat", "ɳChat"},
	{"nself_media_", "media", "nTV"},
	{"nself_family_", "family", "nFamily"},
	{"nself_pro_", "pro", "ɳSelf Pro"},
	{"nself_ent_", "enterprise", "ɳSelf Enterprise"},
	{"nself_max_", "max", "ɳSelf Business+"},
}

// CollectLicenseKeys reads all configured license keys from environment
// variables and the stored key file. It deduplicates and returns all non-empty
// keys. The order is: NSELF_PLUGIN_LICENSE_KEY, then NSELF_LICENSE_KEY_1
// through _10, then the stored key file.
func CollectLicenseKeys() []string {
	seen := make(map[string]bool)
	var keys []string

	add := func(k string) {
		k = strings.TrimSpace(k)
		if k != "" && !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}

	// Legacy single key env var.
	add(os.Getenv("NSELF_PLUGIN_LICENSE_KEY"))

	// Numbered env vars 1-10.
	for i := 1; i <= 10; i++ {
		add(os.Getenv(fmt.Sprintf("NSELF_LICENSE_KEY_%d", i)))
	}

	// Stored key file.
	if fileKey, err := GetKey(); err == nil {
		add(fileKey)
	}

	return keys
}

// ValidateKeyFormat checks that a key has a recognized product prefix and
// meets the minimum length requirement. Returns errs.ErrInvalidLicenseKey on
// failure.
func ValidateKeyFormat(key string) error {
	if len(key) < minKeyLength {
		return errs.ErrInvalidLicenseKey
	}
	for _, pp := range validProductPrefixes {
		if strings.HasPrefix(key, pp.Prefix) {
			return nil
		}
	}
	return fmt.Errorf("unknown key prefix: %w", errs.ErrInvalidLicenseKey)
}

// DetectProduct returns the product info for a key based on its prefix.
// Returns nil if the prefix is not recognized.
func DetectProduct(key string) *ProductPrefix {
	for i := range validProductPrefixes {
		if strings.HasPrefix(key, validProductPrefixes[i].Prefix) {
			return &validProductPrefixes[i]
		}
	}
	return nil
}

// MaskKey returns a masked version of the key showing the first 10 and last 4
// characters. Delegates to the existing maskKey function.
func MaskKey(key string) string {
	return maskKey(key)
}

// DetectTierFromKey returns the human-readable tier name for a key based on
// its prefix (e.g. "nself_owner_" → "Owner"). Returns "Unknown" when the
// prefix is not recognised. Public wrapper for the unexported detectTier in
// manager.go.
func DetectTierFromKey(key string) string {
	for _, pp := range validProductPrefixes {
		if strings.HasPrefix(key, pp.Prefix) {
			return pp.DisplayName
		}
	}
	return "Unknown"
}

// AddKey stores an additional license key. If no keys exist yet, it writes to
// the primary key file. If a key already exists, it stores as a numbered key
// file (key.2, key.3, etc.) in ~/.nself/license/.
func AddKey(key string) error {
	key = strings.TrimSpace(key)
	if err := ValidateKeyFormat(key); err != nil {
		return err
	}

	dir, err := licenseDirPath()
	if err != nil {
		return err
	}
	if err := ensureDir(dir); err != nil {
		return fmt.Errorf("creating license directory: %w", err)
	}

	// Check if primary key file exists.
	primaryPath := dir + "/" + keyFile
	if _, err := os.Stat(primaryPath); os.IsNotExist(err) {
		// No primary key — write as primary.
		return os.WriteFile(primaryPath, []byte(key), 0600)
	}

	// Primary exists. Find next available slot (key.2 through key.10).
	for i := 2; i <= 10; i++ {
		path := fmt.Sprintf("%s/%s.%d", dir, keyFile, i)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return os.WriteFile(path, []byte(key), 0600)
		}
	}

	return fmt.Errorf("maximum of 10 license keys reached")
}

// RemoveKey removes a key by prefix match or product name. Returns the number
// of keys removed.
func RemoveKey(query string) (int, error) {
	dir, err := licenseDirPath()
	if err != nil {
		return 0, err
	}

	keys := collectStoredKeys(dir)
	removed := 0
	var remaining []string

	query = strings.TrimSpace(query)
	queryLower := strings.ToLower(query)

	for _, k := range keys {
		shouldRemove := false

		// Match by key value (prefix match).
		if strings.HasPrefix(k, query) {
			shouldRemove = true
		}

		// Match by product name.
		if !shouldRemove {
			if pp := DetectProduct(k); pp != nil {
				if strings.EqualFold(pp.Product, queryLower) || strings.EqualFold(pp.DisplayName, query) {
					shouldRemove = true
				}
			}
		}

		if shouldRemove {
			removed++
		} else {
			remaining = append(remaining, k)
		}
	}

	if removed == 0 {
		return 0, fmt.Errorf("no matching key found for %q", query)
	}

	// Rewrite key files: clear all, then write remaining.
	if err := clearAllKeyFiles(dir); err != nil {
		return 0, err
	}
	for i, k := range remaining {
		var path string
		if i == 0 {
			path = dir + "/" + keyFile
		} else {
			path = fmt.Sprintf("%s/%s.%d", dir, keyFile, i+1)
		}
		if err := os.WriteFile(path, []byte(k), 0600); err != nil {
			return removed, fmt.Errorf("rewriting key file: %w", err)
		}
	}

	return removed, nil
}

// GetAllStoredKeys returns all keys stored in key files (not env vars).
func GetAllStoredKeys() []string {
	dir, err := licenseDirPath()
	if err != nil {
		return nil
	}
	return collectStoredKeys(dir)
}

// SetKeyReplaceAll replaces all stored keys with a single new key.
// Returns the number of keys that were replaced.
func SetKeyReplaceAll(key string) (int, error) {
	key = strings.TrimSpace(key)
	if err := ValidateKeyFormat(key); err != nil {
		return 0, err
	}

	dir, err := licenseDirPath()
	if err != nil {
		return 0, err
	}

	existing := collectStoredKeys(dir)
	count := len(existing)

	// ensureDir first: clearAllKeyFiles removes paths UNDER dir, so if an older
	// layout left a regular file at dir itself, every remove returns ENOTDIR
	// before the directory is ever repaired. Doing the work before making its
	// precondition true is what produced "remove .../license/key: not a
	// directory" on upgraded machines.
	if err := ensureDir(dir); err != nil {
		return 0, fmt.Errorf("creating license directory: %w", err)
	}

	if err := clearAllKeyFiles(dir); err != nil {
		return 0, err
	}

	path := dir + "/" + keyFile
	if err := os.WriteFile(path, []byte(key), 0600); err != nil {
		return 0, fmt.Errorf("writing license key: %w", err)
	}

	return count, nil
}

// collectStoredKeys reads all key files from the license directory.
func collectStoredKeys(dir string) []string {
	var keys []string

	// Primary key file.
	if data, err := os.ReadFile(dir + "/" + keyFile); err == nil {
		if k := strings.TrimSpace(string(data)); k != "" {
			keys = append(keys, k)
		}
	}

	// Numbered key files key.2 through key.10.
	for i := 2; i <= 10; i++ {
		path := fmt.Sprintf("%s/%s.%d", dir, keyFile, i)
		if data, err := os.ReadFile(path); err == nil {
			if k := strings.TrimSpace(string(data)); k != "" {
				keys = append(keys, k)
			}
		}
	}

	return keys
}
