package main

// registry.go: build registry.json for one tier from manifests, releases.json
// and bundles.json.
//
// Purpose: replace the hand-edited registry (and plugins-pro build-registry.py)
// with a projection. Every key keeps the name and type released CLIs decode
// (v1.4.12 registry_parse.go pluginEntry); `release_signature` and `catalog`
// are additive keys they ignore.
// Inputs: manifests of one tier, releases, bundles, the other tier's slugs.
// Outputs: model.Registry, data problems, and the slugs skipped because
// releases.json has no row for them (never released, so no checksum).
// Constraints: no value is typed by hand: manifest fields come from
// manifestv2, release fields from releases.json, membership from bundles.json,
// tier_pair from the other tier's registry. Base URLs are the served routes.

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/nself-org/cli/internal/plugin/manifestv2"
	"github.com/nself-org/cli/tools/catalog/model"
)

const (
	freeDownload     = "https://plugins.nself.org/plugins/%s/tarball"
	licensedDownload = "https://ping.nself.org/plugins/%s/download"
)

// tierInfo is what differs between the two registries.
type tierInfo struct {
	license  string // manifest license value required in this tree
	wire     string // registry `tier` value
	catalog  string // catalog tier
	download string // download_url format
}

var (
	freeTier     = tierInfo{manifestv2.LicenseFree, model.WireFree, model.TierFree, freeDownload}
	licensedTier = tierInfo{manifestv2.LicenseLicensed, model.WirePro, model.TierLicensed, licensedDownload}
)

// buildRegistry projects the manifests of one tier.
func buildRegistry(t tierInfo, ms map[string]*manifestv2.Manifest, rel *model.Releases,
	b *model.Bundles, peer map[string]bool, from model.GeneratedFrom) (reg *model.Registry, probs, unreleased []string) {
	reg = &model.Registry{
		Generated: model.Generated, GeneratedFrom: from, SchemaVersion: model.RegistryVersion,
		Tier: t.wire, ChecksumAlgorithm: "sha256", Plugins: map[string]model.RegistryEntry{},
	}
	slugs := make([]string, 0, len(ms))
	for s := range ms {
		slugs = append(slugs, s)
	}
	sort.Strings(slugs)
	for _, s := range slugs {
		m := ms[s]
		if m.License != t.license {
			probs = append(probs, fmt.Sprintf("%s: license %q does not belong in the %s tree (a plugin never moves tier here)", s, m.License, t.catalog))
			continue
		}
		r, ok := rel.Releases[s]
		if !ok { // a manifest with no release has no checksum: not in the registry yet
			unreleased = append(unreleased, s)
			continue
		}
		reg.Plugins[s] = entryOf(t, m, r, membership(b, s, t.catalog), peer[s])
	}
	reg.PluginsCount = len(reg.Plugins)
	return reg, probs, unreleased
}

// entryOf projects one manifest and its release row.
func entryOf(t tierInfo, m *manifestv2.Manifest, r model.Release, bundles []string, pair bool) model.RegistryEntry {
	e := model.RegistryEntry{
		Name: m.Name, Version: r.Version, Description: m.Description, Category: m.Category,
		Tier: t.wire, License: firstNonEmpty(m.LicenseSPDX, m.License),
		MinNselfVersion: m.MinNselfVersion, MinNselfVersionSnake: m.MinNselfVersion,
		RequiresLicense: m.RequiresLicense, Language: m.Language,
		Tags: nonNil(m.Tags), Tables: nonNil(m.Tables), Author: m.Author,
		Dependencies: dependencies(t, m), Bundles: bundles, TierPair: pair,
		Checksum: r.SHA256, ReleaseSignature: r.ReleaseSignature,
		TarballURL: r.TarballURL, ReleaseTag: r.ReleaseTag,
		Tarball: firstNonEmpty(r.Tarball, r.TarballURL), Catalog: infoOf(m),
	}
	e.Port = m.Port
	if e.Port == 0 && m.Service != nil && m.Service.Port != nil {
		e.Port = *m.Service.Port
	}
	if m.Installable != nil && !*m.Installable {
		no := false
		e.Installable = &no
	}
	if m.Runtime != "" || m.EntryPoint != "" || m.PluginType != "" || m.BinaryName != "" {
		e.Implementation = &model.Implementation{Language: m.Language, Runtime: m.Runtime,
			EntryPoint: m.EntryPoint, PluginType: m.PluginType, BinaryName: m.BinaryName}
	}
	// A single cliCommands entry repeats binaryName; released CLIs need the
	// list only when a plugin ships more than one command.
	if len(m.CLICommands) > 1 {
		for _, c := range m.CLICommands {
			e.CLICommands = append(e.CLICommands, model.CLICommand{Name: c.Name, Description: c.Description})
		}
	}
	if r.TarballURL != "" || r.Tarball != "" {
		e.DownloadURL = fmt.Sprintf(t.download, m.Name)
	}
	if t.catalog == model.TierFree {
		e.Checksums = &model.Checksums{SHA256: r.SHA256, Platforms: r.PlatformChecksums}
	}
	return e
}

// dependencies keeps the shape each registry has always used: a free entry is a
// plain list unless it has optional dependencies, a licensed entry is always
// {required, optional}.
func dependencies(t tierInfo, m *manifestv2.Manifest) json.RawMessage {
	var v any = nonNil(m.Dependencies)
	if t.catalog == model.TierLicensed || len(m.OptionalDependencies) > 0 {
		v = map[string][]string{"required": nonNil(m.Dependencies), "optional": nonNil(m.OptionalDependencies)}
	}
	raw, _ := json.Marshal(v) // strings and slices only: cannot fail
	return raw
}

// infoOf is the manifest-only part of a catalog row.
func infoOf(m *manifestv2.Manifest) *model.Info {
	in := &model.Info{Maturity: m.Maturity, DocsURL: m.DocsURL}
	if m.Service != nil {
		in.ServiceKind = m.Service.Kind
	}
	if m.Schema != nil {
		in.Schema = *m.Schema
	}
	if m.Requires != nil {
		in.Requires = &model.Requires{Nself: m.Requires.Nself, Plugins: m.Requires.Plugins,
			PostgresExtensions: m.Requires.PostgresExtensions}
	}
	if c := m.Commands; c != nil {
		in.Commands = &model.Commands{Command: c.Command, Subcommands: []string{}}
		for _, s := range c.Subcommands {
			in.Commands.Subcommands = append(in.Commands.Subcommands, s.Name)
		}
		sort.Strings(in.Commands.Subcommands)
	}
	return in
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
