package commands

// Purpose: the catalog and licence source of the clone-template entitlement gate (P7-PLUG-25).
// Inputs: catalogURL (a var so tests can point it at a local server) and every configured licence key.
// Outputs: a clonetemplate.Lookup over the plugin catalog (both tiers) and the licence checks.
// Constraints: no licence key means nothing is covered and no licence call is made. A Bundle is
// checked with license.BundleEntitled; the plugin-level check validates each key once and accepts
// an all-access tier or a plugin the licence names, the same grants plugin install honours.

import (
	"context"
	"slices"

	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/plugin/catalog"
	clonetemplate "github.com/nself-org/cli/internal/templates/clone"
)

// templateSource is replaced in tests; the default needs the network.
var templateSource clonetemplate.Source = func(ctx context.Context) (clonetemplate.Lookup, clonetemplate.Entitlement, error) {
	res, err := catalog.Fetch(ctx, catalogURL, catalog.DefaultCacheDir())
	if err != nil {
		return nil, clonetemplate.Entitlement{}, err
	}
	find := func(slug string) (clonetemplate.PluginInfo, bool) {
		e, ok := res.Catalog.Lookup(slug)
		return clonetemplate.PluginInfo{Slug: e.Slug, Tier: e.Tier, Bundles: e.Bundles}, ok
	}
	keys := templateLicenseKeys()
	validated := map[string]*license.ValidationResult{}
	return find, clonetemplate.Entitlement{
		Bundle: func(ctx context.Context, bundle string) (bool, error) {
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
		},
		Plugin: func(ctx context.Context, slug string) (bool, error) {
			var firstErr error
			for _, key := range keys {
				res, seen := validated[key]
				if !seen {
					var err error
					if res, err = license.ValidateFull(ctx, key); err != nil && firstErr == nil {
						firstErr = err
					}
					validated[key] = res
				}
				if res != nil && res.Valid && (license.IsAllAccessTier(res.Tier) || slices.Contains(res.Plugins, slug)) {
					return true, nil
				}
			}
			return false, firstErr
		},
	}, nil
}

// templateLicenseKeys is every configured licence key, the owner key first, so
// coverage under any of them counts (multi-key support, as plugin install).
func templateLicenseKeys() []string {
	keys := license.CollectLicenseKeys()
	if k := license.CollectLicenseKey(); k != "" && !slices.Contains(keys, k) {
		keys = append([]string{k}, keys...)
	}
	return keys
}
