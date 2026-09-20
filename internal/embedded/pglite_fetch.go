package embedded

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/cli/internal/httptimeout"
)

const (
	// pgliteBaseURL is the primary CDN endpoint served by ping.nself.org (CS_1).
	pgliteBaseURL = "https://ping.nself.org/assets/pglite"

	// pgliteFallbackBaseURL is the upstream npm CDN used when ping.nself.org is
	// unreachable. The npm package ships the artifact as postgres.wasm; the
	// fallback URL maps it to the same pglite.wasm filename convention.
	// Pattern: https://registry.npmjs.org/@electric-sql/pglite/-/pglite-<ver>.tgz
	// We use the direct unpkg path which resolves the same content without tarball extraction.
	pgliteFallbackBaseURL = "https://registry.npmjs.org/@electric-sql/pglite/-/pglite" //nolint:unused // kept: pglite fallback URL never consulted; see qa/bugs/declared-but-never-wired-symbols.md

	// cacheMaxAge is how long a cached WASM file is trusted without re-verifying
	// its SHA-256 digest against the pinned value.
	cacheMaxAge = 7 * 24 * time.Hour

	// downloadTimeout caps the time allowed for the WASM artifact download.
	downloadTimeout = 120 * time.Second
)

// pgliteBaseURLOverride can be set by tests to redirect primary CDN downloads to
// a local httptest.Server without modifying the pgliteBaseURL constant.
var pgliteBaseURLOverride string

// pgliteFallbackURLOverride can be set by tests to redirect fallback CDN
// downloads to a local httptest.Server. When nil, pgliteFallbackURL is used.
var pgliteFallbackURLOverride func(version string) string

// activePgliteBaseURL returns the effective CDN base URL (primary).
func activePgliteBaseURL() string {
	if pgliteBaseURLOverride != "" {
		return pgliteBaseURLOverride
	}
	return pgliteBaseURL
}

// pgliteFallbackURL returns the upstream npm CDN URL for a given version.
// The npm package ships the artifact as postgres.wasm; the content is identical
// to the pglite.wasm artifact served by ping.nself.org.
func pgliteFallbackURL(version string) string {
	if pgliteFallbackURLOverride != nil {
		return pgliteFallbackURLOverride(version)
	}
	// unpkg resolves package internals without client-side tarball extraction.
	return fmt.Sprintf("https://unpkg.com/@electric-sql/pglite@%s/dist/postgres.wasm", version)
}

// FetchOrCached returns the absolute path to the pglite.wasm artifact for the
// given version. It checks the local cache first; if the artifact is absent or
// its digest does not match the pinned value it downloads and re-caches it.
//
// Download order:
//  1. ping.nself.org/assets/pglite/<ver>/pglite.wasm (primary CDN)
//  2. unpkg.com/@electric-sql/pglite@<ver>/dist/postgres.wasm (upstream fallback)
//
// Both sources are verified against the same pinned SHA-256 before caching.
// The cache directory is ~/.nself/cache/pglite/<version>/.
// The artifact is written atomically (tmp -> rename) so partial downloads never
// leave a corrupt cached copy.
//
// FetchOrCached returns an error if:
//   - version is not in sha256Pins
//   - neither source yields an artifact whose SHA-256 matches the pin
//   - any non-network I/O operation fails
func FetchOrCached(ctx context.Context, version string) (string, error) {
	pin, ok := sha256Pins[version]
	if !ok {
		return "", fmt.Errorf("embedded/pglite: unknown version %q — add a sha256 pin before using", version)
	}

	cacheDir, err := pgliteCacheDir(version)
	if err != nil {
		return "", err
	}

	cachedPath := filepath.Join(cacheDir, "pglite.wasm")

	// Fast path: cache hit with valid digest.
	if ok, _ := fileMatchesDigest(cachedPath, pin, cacheMaxAge); ok {
		return cachedPath, nil
	}

	// Slow path: download from primary CDN with fallback.
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return "", fmt.Errorf("embedded/pglite: create cache dir: %w", err)
	}

	primaryURL := fmt.Sprintf("%s/%s/pglite.wasm", activePgliteBaseURL(), version)
	primaryErr := downloadAndVerify(ctx, primaryURL, cachedPath, pin)
	if primaryErr == nil {
		return cachedPath, nil
	}

	// Primary CDN unreachable or returned wrong content — try upstream npm CDN.
	fallbackURL := pgliteFallbackURL(version)
	if fallbackErr := downloadAndVerify(ctx, fallbackURL, cachedPath, pin); fallbackErr != nil {
		// Return the primary error as it is the expected source; include fallback error for context.
		return "", fmt.Errorf("embedded/pglite: primary CDN failed (%w); fallback also failed: %v", primaryErr, fallbackErr)
	}

	return cachedPath, nil
}

// pgliteCacheDir returns the version-specific cache directory path.
// It resolves the home directory via os.UserHomeDir so it works correctly
// under sudo and in CI environments.
func pgliteCacheDir(version string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("embedded/pglite: resolve home dir: %w", err)
	}
	return filepath.Join(home, ".nself", "cache", "pglite", version), nil
}

// fileMatchesDigest reports whether path exists, is not a directory, has a
// modification time within maxAge of now, and its SHA-256 hex digest equals
// want. It returns (false, nil) for any condition that warrants a re-download
// rather than an error.
func fileMatchesDigest(path, want string, maxAge time.Duration) (bool, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return false, nil // absent or inaccessible — trigger download
	}
	if fi.IsDir() {
		return false, nil
	}
	if time.Since(fi.ModTime()) > maxAge {
		return false, nil // stale — re-verify
	}

	got, err := sha256HexFile(path)
	if err != nil {
		// KEEP, same contract as the branches above: every "no" here means
		// re-download, and a cached file we cannot hash is exactly as useless
		// as one that is absent or stale. Nothing is lost by re-fetching, and
		// downloadAndVerify checks the digest again on the fresh copy, so a
		// genuinely broken cache path surfaces there with the write/verify
		// error rather than being retried forever in silence.
		return false, nil
	}
	return got == want, nil
}

// downloadAndVerify fetches url into a temp file in the same directory as dst,
// verifies its SHA-256 digest against want, then atomically replaces dst.
func downloadAndVerify(ctx context.Context, url, dst, want string) error {
	dlCtx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("embedded/pglite: build request: %w", err)
	}

	resp, err := httptimeout.Installer.Do(req)
	if err != nil {
		return fmt.Errorf("embedded/pglite: download %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("embedded/pglite: server returned %d for %s", resp.StatusCode, url)
	}

	// Write to a sibling temp file so rename is atomic on the same filesystem.
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".pglite-download-*")
	if err != nil {
		return fmt.Errorf("embedded/pglite: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // no-op after successful rename

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp.Body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("embedded/pglite: write download: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("embedded/pglite: close temp file: %w", err)
	}

	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		return fmt.Errorf("embedded/pglite: sha256 mismatch for version: got %s, want %s", got, want)
	}

	if err := os.Rename(tmpPath, dst); err != nil {
		return fmt.Errorf("embedded/pglite: atomic rename: %w", err)
	}
	return nil
}

// sha256HexFile computes the SHA-256 hex digest of the file at path.
func sha256HexFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
