package bundle

// Purpose: identify installed free tier-pair members of paid bundles so a
// successful install can explain when the final copy remains free.
// Inputs: the member plan, installed manifests, and the registry cache.
// Outputs: names requiring a final tier check, or a registry error.
// Constraints: checks the registry before mutations; never changes plugin tier.

import (
	"context"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/plugin"
)

func (m *memberRun) retainedFreePairs(ctx context.Context) (map[string]bool, error) {
	pairs := make(map[string]bool)
	if m.b.IsFree() {
		return pairs, nil
	}
	var freeNames []string
	for _, name := range m.planned {
		if tier, _, ok := plugin.InstalledTier(m.pluginDir, name); ok && tier == plugin.LicenseFree {
			freeNames = append(freeNames, name)
		}
	}
	if len(freeNames) == 0 {
		return pairs, nil
	}
	reg, err := plugin.FetchRegistry(ctx, "", defaultRegistryCacheDir())
	if err != nil {
		return nil, fmt.Errorf("checking retained bundle plugin tiers: %w", err)
	}
	for _, name := range freeNames {
		var matches int
		var free, licensed bool
		for _, entry := range reg.Plugins {
			if !strings.EqualFold(entry.Name, name) {
				continue
			}
			matches++
			if entry.TierPair {
				free = free || entry.Tier == plugin.WireTierFree
				licensed = licensed || entry.Tier == plugin.WireTierPro
			}
		}
		pairs[name] = matches == 2 && free && licensed
	}
	return pairs, nil
}
