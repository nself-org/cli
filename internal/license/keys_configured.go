package license

import (
	"slices"
	"strings"
)

// ConfiguredKeys is every configured licence key, the owner key first, so
// coverage under any of them counts (multi-key support, as plugin install).
func ConfiguredKeys() []string {
	keys := CollectLicenseKeys()
	// CollectLicenseKeys reads only the primary key file; key.2 to key.10 are
	// stored keys too, and a covered second key must not read as E110.
	for _, k := range GetAllStoredKeys() {
		if k = strings.TrimSpace(k); k != "" && !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	if k := strings.TrimSpace(CollectLicenseKey()); k != "" {
		keys = append([]string{k}, slices.DeleteFunc(keys, func(x string) bool { return x == k })...)
	}
	return keys
}
