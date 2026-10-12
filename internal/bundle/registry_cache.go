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

// loadBundlesFromCache installs the last-known-good cached catalog after
// re-validating its member names (a cache file is as untrusted as a fetch).
// cause is why the live document was not used. invalid is true when the fetch
// succeeded but the document failed validation: that always prints one stderr
// warning naming the failure. For a failed fetch, the warning is printed once
// per process and only when the cache is older than bundlesStaleWarnAfter.
// Returns (true, nil) when the cache was installed; (false, nil) when there is
// no cache file; (false, err) when the cache exists but cannot be used. It
// never writes the cache.
func loadBundlesFromCache(cause error, invalid bool) (bool, error) {
	cached, err := readBundlesCache()
	if err != nil || cached == nil {
		return false, nil
	}
	if err := validateCatalogNames(cached.Doc); err != nil {
		return false, fmt.Errorf("cached bundles.json: %w", err)
	}
	age := time.Since(time.Unix(cached.FetchedAt, 0))
	switch {
	case invalid:
		fmt.Fprintf(os.Stderr,
			"warning: fetched bundles.json failed validation (%v); using the local cache from %s ago.\n",
			cause, age.Round(time.Minute))
	case age > bundlesStaleWarnAfter:
		staleWarnOnce.Do(func() {
			fmt.Fprintf(os.Stderr,
				"warning: bundles.json cache is %s stale (last fetch failed: %v); bundle membership may be outdated. Reconnect and retry.\n",
				age.Round(time.Hour), cause)
		})
	}
	setState(buildBundleMap(cached.Doc))
	return true, nil
}
