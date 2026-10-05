package main

// bundles.go: read bundles.json and answer membership questions.
//
// Purpose: bundles.json is the membership source of truth (ADR 0008); a bundle
// has a license axis (free or paid there, free or licensed in the catalog) and
// applies to the plugins of the matching tier only.
// Inputs: the path of bundles.json. Outputs: model.Bundles, membership lists.
// Constraints: a bundle tier other than free or paid is an error.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/nself-org/cli/tools/catalog/model"
)

// loadBundles reads the file at path.
func loadBundles(path string) (*model.Bundles, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("bundles: %w", err)
	}
	var b model.Bundles
	// bundles.json carries prices and page names the catalog does not read.
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&b); err != nil {
		return nil, fmt.Errorf("bundles %s: %w", path, err)
	}
	for _, k := range sortedKeys(b.Bundles) {
		if _, err := catalogTier(b.Bundles[k].Tier); err != nil {
			return nil, fmt.Errorf("bundles %s: bundle %s: %w", path, k, err)
		}
	}
	return &b, nil
}

// catalogTier maps a bundles.json tier to the catalog license axis.
func catalogTier(t string) (string, error) {
	switch t {
	case "free":
		return model.TierFree, nil
	case "paid":
		return model.TierLicensed, nil
	}
	return "", fmt.Errorf("tier %q is not free or paid", t)
}

// membership returns the sorted bundle keys that list slug for the given
// catalog tier. A free bundle applies to the free entry, a paid bundle to the
// licensed entry.
func membership(b *model.Bundles, slug, tier string) []string {
	var out []string
	for _, k := range sortedKeys(b.Bundles) {
		bt, err := catalogTier(b.Bundles[k].Tier)
		if err != nil || bt != tier {
			continue
		}
		for _, p := range b.Bundles[k].Plugins {
			if p == slug {
				out = append(out, k)
				break
			}
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
