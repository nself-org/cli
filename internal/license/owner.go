// Package license — owner.go implements owner-key separation.
//
// S133-T01: Owner license stored at ~/.nself/owner.license (separate from
//
//	regular license.json / license/key). --owner flag required for
//	owner-key plugin installs.
//
// S133-T02: Refuse owner key from being set when NSELF_ENV=dev. Print error
//
//	message and return without saving.
package license

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ownerLicenseFile is the filename for the owner license key.
const ownerLicenseFile = "owner.license"

// ownerLicensePath returns the path to ~/.nself/owner.license.
func ownerLicensePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determining home directory: %w", err)
	}
	return filepath.Join(home, LicenseDirV1, ownerLicenseFile), nil
}

// IsOwnerKey returns true if the given key uses the nself_owner_ prefix.
func IsOwnerKey(key string) bool {
	return strings.HasPrefix(key, "nself_owner_")
}

// SetOwnerKey saves an owner license key to ~/.nself/owner.license (0600).
//
// Enforces S133-T02: refuses to save if NSELF_ENV=dev to prevent accidental
// owner-key commits via tracked env files.
func SetOwnerKey(key string) error {
	key = strings.TrimSpace(key)
	if !IsOwnerKey(key) {
		return fmt.Errorf("owner key must use the nself_owner_ prefix")
	}

	// S133-T02: refuse if running in dev environment.
	if env := os.Getenv("NSELF_ENV"); strings.EqualFold(env, "dev") {
		return fmt.Errorf(
			"owner keys cannot be saved when NSELF_ENV=dev — " +
				"use NSELF_PLUGIN_LICENSE_KEY for dev testing, " +
				"or unset NSELF_ENV to save the owner key on a non-dev machine",
		)
	}

	path, err := ownerLicensePath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := ensureDir(dir); err != nil {
		return fmt.Errorf("creating owner license directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(key), 0600); err != nil {
		return fmt.Errorf("writing owner license: %w", err)
	}
	return nil
}

// GetOwnerKey reads the stored owner license key.
// Returns ("", nil) if no owner key is configured.
func GetOwnerKey() (string, error) {
	// Env var takes precedence (for testing / CI).
	if envKey := os.Getenv("NSELF_OWNER_LICENSE_KEY"); envKey != "" {
		return strings.TrimSpace(envKey), nil
	}

	path, err := ownerLicensePath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// KEEP. Absent owner-licence file means no owner key is
			// configured, which is the normal state for every install that is
			// not ours. Only ENOENT is folded in; a read that fails for any
			// other reason is returned, so an unreadable owner key is never
			// reported as "not an owner".
			return "", nil
		}
		return "", fmt.Errorf("reading owner license: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// ClearOwnerKey removes the owner license file.
func ClearOwnerKey() error {
	path, err := ownerLicensePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing owner license: %w", err)
	}
	return nil
}
