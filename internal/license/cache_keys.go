package license

import "github.com/nself-org/cli/internal/compat"

// Purpose: retain one cache entry per key while preserving the legacy head.
// Inputs: new entry. Outputs: independent cache snapshot. Constraints: v1.5 only.
// SPORT: plugin.entitlement-token v1.1.
func cacheWithHistory(entry *CacheEntry) *CacheEntry {
	if !compat.V15() || entry == nil {
		return entry
	}
	result := *entry
	result.Entries = make(map[string]*CacheEntry)
	if old, err := ReadCache(); err == nil && old != nil {
		for hash, previous := range old.Entries {
			if previous != nil {
				copyEntry := *previous
				copyEntry.Entries = nil
				result.Entries[hash] = &copyEntry
			}
		}
		old.Entries = nil
		result.Entries[old.KeyHash] = old
	}
	delete(result.Entries, entry.KeyHash)
	return &result
}

// Purpose: read the cache for a particular configured key.
// Inputs: key or empty for legacy head. Outputs: entry/error. Constraints: callers
// still verify signatures and subject binding. SPORT: per-key offline continuity.
func ReadCacheForKey(key string) (*CacheEntry, error) {
	entry, err := ReadCache()
	if err != nil || entry == nil || key == "" || !compat.V15() || entry.KeyHash == HashKey(key) {
		return entry, err
	}
	return entry.Entries[HashKey(key)], nil
}
