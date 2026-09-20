// registry_cache.go — local on-disk cache for the last-known-good
// bundles.json (P6-E4-W3-S3-T10). Split out of registry.go to stay under the
// 300-line/file cap (internal/repoqa).
package bundle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// bundlesCachePath returns the on-disk cache location, honoring
// NSELF_BUNDLES_CACHE_PATH (mirrors license.CachePath's LICENSE_CACHE_PATH
// override convention).
func bundlesCachePath() (string, error) {
	if p := os.Getenv("NSELF_BUNDLES_CACHE_PATH"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determining home directory: %w", err)
	}
	return filepath.Join(home, ".cache", "nself", bundlesCacheFileName), nil
}

// writeBundlesCache atomically writes a freshly fetched bundles.json payload
// to the local cache (tmpfile + rename, matching license.WriteCache).
func writeBundlesCache(rawDoc []byte) error {
	var doc bundlesDoc
	if err := json.Unmarshal(rawDoc, &doc); err != nil {
		return err
	}
	entry := cachedBundlesDoc{FetchedAt: time.Now().Unix(), Doc: doc}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	path, err := bundlesCachePath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating bundles cache directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".bundles.json.tmp.*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

// readBundlesCache reads the last-known-good cached bundles.json. Returns
// nil, nil if no cache file exists.
func readBundlesCache() (*cachedBundlesDoc, error) {
	path, err := bundlesCachePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// KEEP. A cold cache has no last-known-good document; the caller
			// falls back to a live fetch, which is what it would do for an
			// error too. ENOENT only, so a corrupt or unreadable cache still
			// surfaces instead of masquerading as a first run.
			return nil, nil
		}
		return nil, fmt.Errorf("reading bundles cache: %w", err)
	}
	var entry cachedBundlesDoc
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("parsing bundles cache: %w", err)
	}
	return &entry, nil
}
