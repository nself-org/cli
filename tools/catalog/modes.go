package main

// modes.go: the three run modes, wired from flags to the pure builders.
//
// Purpose: read inputs, call buildRegistry or buildCatalog, render outputs.
// Inputs: parsed options. Outputs: rendered files plus the list of problems.
// Constraints: a missing required flag is a returned error (exit 2), never a
// panic; problems are data problems and are all reported.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/tools/catalog/model"
)

func need(flag, val string) error {
	if val == "" {
		return fmt.Errorf("%s is required", flag)
	}
	return nil
}

// runTier generates one tier's registry.json.
func runTier(o *options, t tierInfo, stderr io.Writer) (outputs, []string, error) {
	for _, f := range [][2]string{{"-plugins", o.plugins}, {"-releases", o.releases}, {"-bundles", o.bundles}} {
		if err := need(f[0], f[1]); err != nil {
			return nil, nil, err
		}
	}
	rel, err := loadReleases(o.releases)
	if err != nil {
		return nil, nil, err
	}
	b, err := loadBundles(o.bundles)
	if err != nil {
		return nil, nil, err
	}
	var peer map[string]bool
	if o.peer != "" {
		r, err := readRegistry(o.peer)
		if err != nil {
			return nil, nil, err
		}
		peer = map[string]bool{}
		for s := range r.Plugins {
			peer[s] = true
		}
	}
	ms, probs := scanManifests(o.plugins)
	for _, s := range sortedKeys(rel.Releases) { // a release row needs a plugin directory
		if _, err := os.Stat(filepath.Join(o.plugins, s)); err != nil {
			probs = append(probs, s+": releases.json has a row but the tree has no such directory")
		}
	}
	reg, rp, unreleased := buildRegistry(t, ms, rel, b, peer, o.from)
	for _, s := range unreleased {
		say(stderr, "catalog: note: %s has no row in releases.json; left out of the registry\n", s)
	}
	out := outputs{}
	if err := out.add("registry.json", reg); err != nil {
		return nil, nil, err
	}
	return out, append(probs, rp...), nil
}

// runCatalog generates catalog.json and counts.json.
func runCatalog(o *options) (outputs, []string, error) {
	for _, f := range [][2]string{{"-free-registry", o.freeReg}, {"-licensed-registry", o.licReg}, {"-bundles", o.bundles}} {
		if err := need(f[0], f[1]); err != nil {
			return nil, nil, err
		}
	}
	free, err := readRegistry(o.freeReg)
	if err != nil {
		return nil, nil, err
	}
	lic, err := readRegistry(o.licReg)
	if err != nil {
		return nil, nil, err
	}
	b, err := loadBundles(o.bundles)
	if err != nil {
		return nil, nil, err
	}
	cat, probs := buildCatalog(free, lic, b, o.from)
	out := outputs{}
	if err := out.add("catalog.json", cat); err != nil {
		return nil, nil, err
	}
	if err := out.add("counts.json", cat.Counts); err != nil {
		return nil, nil, err
	}
	return out, probs, nil
}

// readRegistry decodes a registry.json leniently: the basis files carry keys
// this tool does not read.
func readRegistry(path string) (*model.Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("registry: %w", err)
	}
	var r model.Registry
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("registry %s: %w", path, err)
	}
	return &r, nil
}
