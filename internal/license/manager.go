// Package license provides CLI-level license key management operations:
// setting, reading, clearing, and displaying license keys.
//
// Key storage: ~/.config/nself/license.json (chmod 0600)
// Legacy storage (v1): ~/.nself/license.json
// Env override: NSELF_PLUGIN_LICENSE_KEY
package license

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// LicenseDirV1 is the v1 license directory name under $HOME.
const LicenseDirV1 = ".nself"

// LicenseDirV2 is the v2 license directory name under $HOME.
const LicenseDirV2 = ".config/nself"

// LicenseFile is the license JSON filename used in both v1 and v2.
const LicenseFile = "license.json"

// licenseDir is the subdirectory under the user home where license files live.
const licenseDir = ".nself/license"

// keyFile is the filename for the stored license key.
const keyFile = "key"

// cacheFile is the filename for the license validation cache.
const cacheFile = "cache"

// minKeyLength is the minimum accepted license key length.
const minKeyLength = 32

// validPrefixes lists the accepted license key prefixes and their tier names.
var validPrefixes = []struct {
	Prefix string
	Tier   string
}{
	{"nself_owner_", "Owner"},
	{"nself_ent_", "Enterprise"},
	{"nself_max_", "Business+"},
	{"nself_pro_", "Pro"},
}

// licenseDirPath returns the absolute path to ~/.nself/license/.
func licenseDirPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determining home directory: %w", err)
	}
	return filepath.Join(home, licenseDir), nil
}

// SetKey validates the key format, creates the license directory if needed,
// and writes the key to ~/.nself/license/key with mode 0600.
func SetKey(key string) error {
	key = strings.TrimSpace(key)
	if err := validateFormat(key); err != nil {
		return err
	}

	dir, err := licenseDirPath()
	if err != nil {
		return err
	}
	if err := ensureDir(dir); err != nil {
		return fmt.Errorf("creating license directory: %w", err)
	}

	path := filepath.Join(dir, keyFile)
	if err := os.WriteFile(path, []byte(key), 0600); err != nil {
		return fmt.Errorf("writing license key: %w", err)
	}
	return nil
}

// GetKey returns the current license key. It checks the NSELF_PLUGIN_LICENSE_KEY
// environment variable first, then falls back to reading ~/.nself/license/key.
// Returns ("", nil) when no key is configured.
func GetKey() (string, error) {
	if envKey := os.Getenv("NSELF_PLUGIN_LICENSE_KEY"); envKey != "" {
		return strings.TrimSpace(envKey), nil
	}

	dir, err := licenseDirPath()
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(filepath.Join(dir, keyFile))
	if err != nil {
		if os.IsNotExist(err) {
			// KEEP. "No key file" IS "no key configured" — there is no third
			// state for a caller to get wrong, and the alternative (an error)
			// would make the unlicensed free tier, which is the majority of
			// installs, look broken.
			//
			// This is narrow on purpose: os.IsNotExist matches ENOENT only.
			// A permission error, a corrupt mount or an ENOTDIR from a
			// ~/.nself/license that is a regular file still comes back as an
			// error below, so "I could not read your licence" never gets
			// downgraded to "you do not have one" — the failure mode that
			// would silently deny a paying customer their entitlement.
			return "", nil
		}
		return "", fmt.Errorf("reading license key: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// ClearKey removes both the stored license key file and the validation cache
// file from ~/.nself/license/.
func ClearKey() error {
	dir, err := licenseDirPath()
	if err != nil {
		return err
	}

	for _, name := range []string{keyFile, cacheFile} {
		path := filepath.Join(dir, name)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing %s: %w", name, err)
		}
	}
	return nil
}

// ShowKey returns a masked version of the current key and its detected tier.
// The masked key shows the first 10 and last 4 characters with the middle
// replaced by asterisks. If no key is set, all return values are empty strings
// with a nil error.
func ShowKey() (masked string, tier string, err error) {
	key, err := GetKey()
	if err != nil {
		return "", "", err
	}
	if key == "" {
		return "", "", nil
	}

	masked = maskKey(key)
	tier = detectTier(key)
	return masked, tier, nil
}

// validateFormat checks that a key has a recognized prefix and meets the
// minimum length requirement.
func validateFormat(key string) error {
	if len(key) < minKeyLength {
		return errs.ErrInvalidLicenseKey
	}
	for _, vp := range validPrefixes {
		if strings.HasPrefix(key, vp.Prefix) {
			return nil
		}
	}
	return errs.ErrInvalidLicenseKey
}

// detectTier returns the human-readable tier name for a key based on its
// prefix. Returns "Unknown" if the prefix is not recognized.
func detectTier(key string) string {
	for _, vp := range validPrefixes {
		if strings.HasPrefix(key, vp.Prefix) {
			return vp.Tier
		}
	}
	return "Unknown"
}

// maskKey returns a masked version of the key showing the first 10 and last 4
// characters. Keys shorter than 15 characters are returned with only the first
// 4 characters visible.
func maskKey(key string) string {
	const showStart = 10
	const showEnd = 4

	if len(key) <= showStart+showEnd {
		// Very short key — show less to avoid exposing the whole thing.
		if len(key) <= showEnd {
			return strings.Repeat("*", len(key))
		}
		return key[:4] + strings.Repeat("*", len(key)-4)
	}

	middle := len(key) - showStart - showEnd
	return key[:showStart] + strings.Repeat("*", middle) + key[len(key)-showEnd:]
}

// MigrateLicenseFromV1 copies ~/.nself/license.json to ~/.config/nself/license.json
// when upgrading from v1. The function is a no-op when:
//   - v2 already exists (NEVER overwrite existing v2 license)
//   - v1 does not exist (nothing to migrate)
//
// On success the v1 file is preserved (non-destructive). File is written with
// mode 0600 matching the key file convention.
func MigrateLicenseFromV1(home string) error {
	v1Path := filepath.Join(home, LicenseDirV1, LicenseFile)
	v2Dir := filepath.Join(home, LicenseDirV2)
	v2Path := filepath.Join(v2Dir, LicenseFile)

	// Skip if v2 already exists — never overwrite (TRAP-04).
	if _, err := os.Stat(v2Path); err == nil {
		return nil
	}

	// Skip if v1 doesn't exist — nothing to migrate.
	if _, err := os.Stat(v1Path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("checking v1 license: %w", err)
	}

	// Ensure v2 directory exists.
	if err := ensureDir(v2Dir); err != nil {
		return fmt.Errorf("creating v2 license directory: %w", err)
	}

	// Open v1 source.
	src, err := os.Open(v1Path)
	if err != nil {
		return fmt.Errorf("opening v1 license: %w", err)
	}
	defer func() { _ = src.Close() }()

	// Create v2 destination with restricted permissions.
	dst, err := os.OpenFile(v2Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("creating v2 license file: %w", err)
	}
	defer func() { _ = dst.Close() }()

	if _, err := io.Copy(dst, src); err != nil {
		// Remove incomplete destination on copy failure.
		_ = os.Remove(v2Path)
		return fmt.Errorf("copying license to v2: %w", err)
	}

	fmt.Fprintf(os.Stderr, "info: migrated license from %s to %s\n", v1Path, v2Path)
	return nil
}
