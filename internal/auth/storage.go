// Package auth provides CLI authentication storage and client utilities.
// ~/.nself/auth.json is the credential store with 0600 permissions.
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// authFileName is the credential file name within ~/.nself/
	authFileName = "auth.json"
	// authFileMode ensures only the owner can read/write the file (0600)
	authFileMode = 0600
	// authDirMode for the parent directory
	authDirMode = 0700
)

// ErrNotLoggedIn is returned when no auth.json exists or token is missing.
var ErrNotLoggedIn = errors.New("not logged in — run 'nself login' to authenticate")

// AuthFile holds the persisted auth credentials.
type AuthFile struct {
	// SessionToken is the opaque session token for server-side session lookup.
	SessionToken string `json:"session_token"`
	// AccessToken is the RS256 JWT for subapp requests.
	AccessToken string `json:"access_token"`
	// Email is the account email (display only — not used for auth).
	Email string `json:"email"`
	// Tier is the account tier (display only).
	Tier string `json:"tier"`
	// DisplayName is the account display name (display only).
	DisplayName string `json:"display_name,omitempty"`
	// Bundles are the unlocked plugin bundles (display only).
	Bundles []string `json:"bundles,omitempty"`
	// ExpiresAt is the access token expiry as RFC3339 string.
	ExpiresAt string `json:"expires_at,omitempty"`
}

// authFilePath returns the absolute path to ~/.nself/auth.json.
func authFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".nself", authFileName), nil
}

// ReadAuthFile reads and parses ~/.nself/auth.json.
// Returns ErrNotLoggedIn if file does not exist.
func ReadAuthFile() (*AuthFile, error) {
	path, err := authFilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotLoggedIn
		}
		return nil, fmt.Errorf("reading auth file: %w", err)
	}

	var af AuthFile
	if err := json.Unmarshal(data, &af); err != nil {
		return nil, fmt.Errorf("parsing auth file: %w", err)
	}

	if af.AccessToken == "" {
		return nil, ErrNotLoggedIn
	}

	return &af, nil
}

// WriteAuthFile writes auth credentials to ~/.nself/auth.json with 0600 perms.
// Creates ~/.nself/ directory with 0700 if it does not exist.
func WriteAuthFile(af *AuthFile) error {
	path, err := authFilePath()
	if err != nil {
		return err
	}

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, authDirMode); err != nil {
		return fmt.Errorf("creating .nself directory: %w", err)
	}

	data, err := json.MarshalIndent(af, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding auth file: %w", err)
	}

	// WriteFile with explicit 0600 ensures correct perms even if file already exists
	if err := os.WriteFile(path, data, authFileMode); err != nil {
		return fmt.Errorf("writing auth file: %w", err)
	}

	// Explicitly enforce 0600 in case umask overrides WriteFile
	if err := os.Chmod(path, authFileMode); err != nil {
		return fmt.Errorf("setting auth file permissions: %w", err)
	}

	return nil
}

// DeleteAuthFile removes ~/.nself/auth.json if it exists.
// Returns nil if the file does not exist (idempotent).
func DeleteAuthFile() error {
	path, err := authFilePath()
	if err != nil {
		return err
	}

	err = os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("deleting auth file: %w", err)
	}

	return nil
}

// IsLoggedIn returns true if ~/.nself/auth.json exists and has a token.
func IsLoggedIn() bool {
	af, err := ReadAuthFile()
	return err == nil && af != nil && af.AccessToken != ""
}

// GetAuthFilePath returns the path for use in help text (never reads content).
func GetAuthFilePath() string {
	path, err := authFilePath()
	if err != nil {
		return "~/.nself/auth.json"
	}
	return path
}

// CachedAccountJSON returns the legacy bare-JSON map for a cached auth file.
// It lives here (not in cmd/commands) so the commands package stays under its
// size ceiling; callers encode it themselves.
func CachedAccountJSON(af *AuthFile) map[string]interface{} {
	tier := af.Tier
	if tier == "" {
		tier = "free"
	}
	return map[string]interface{}{
		"email":        af.Email,
		"display_name": af.DisplayName,
		"tier":         tier,
		"bundles":      af.Bundles,
		"cached":       true,
	}
}
