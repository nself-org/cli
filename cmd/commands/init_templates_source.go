package commands

// Purpose: the catalog and licence source of the clone-template entitlement gate (P7-PLUG-25).
// Inputs: catalogURL (a var so tests can point it at a local server) and every configured licence key.
// Outputs: a clonetemplate.Lookup over the plugin catalog (both tiers) and a Bundle entitlement check.
// Constraints: only license.BundleEntitled decides (signed, bound response, bounded offline grace,
// fail closed); nothing here reads or writes the licence cache. A Bundle is covered when any
// configured key covers it. No licence key means nothing is covered and no licence call is made.

import (
	"context"
	"slices"
	"strings"

	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/plugin/catalog"
	clonetemplate "github.com/nself-org/cli/internal/templates/clone"
)

// templateSource is replaced in tests; the default needs the network.
var templateSource clonetemplate.Source = func(ctx context.Context) (clonetemplate.Lookup, clonetemplate.Entitled, error) {
	res, err := catalog.Fetch(ctx, catalogURL, catalog.DefaultCacheDir())
	if err != nil {
		return nil, nil, err
	}
	find := func(slug string) (clonetemplate.PluginInfo, bool) {
		e, ok := res.Catalog.Lookup(slug)
		return clonetemplate.PluginInfo{Slug: e.Slug, Tier: e.Tier, Bundles: e.Bundles}, ok
	}
	keys := templateLicenseKeys()
	return find, func(ctx context.Context, bundle string) (bool, error) {
		var firstErr error
		for _, key := range keys {
			ok, err := license.BundleEntitled(ctx, key, bundle)
			if ok {
				return true, nil
			}
			if err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return false, firstErr
	}, nil
}

// templateLicenseKeys is every configured licence key, the owner key first, so
// coverage under any of them counts (multi-key support, as plugin install).
func templateLicenseKeys() []string {
	keys := license.CollectLicenseKeys()
	for _, key := range license.GetAllStoredKeys() {
		if !slices.Contains(keys, key) {
			keys = append(keys, key)
		}
	}
	if k := strings.TrimSpace(license.CollectLicenseKey()); k != "" {
		keys = append([]string{k}, slices.DeleteFunc(keys, func(x string) bool { return x == k })...)
	}
	return keys
}
