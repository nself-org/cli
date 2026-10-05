package main

// registry.go: build registry.json for one tier from manifests, releases.json
// and bundles.json.
//
// Purpose: replace the hand-edited registry (and plugins-pro build-registry.py)
// with a projection. Every key keeps the name and type released CLIs decode
// (v1.4.12 registry_parse.go pluginEntry); `release_signature` and `catalog`
// are additive keys they ignore.
// Inputs: manifests of one tier, releases, bundles, the other tier's slugs.
// Outputs: model.Registry and a report of problems and notes.
// Constraints: no value is typed by hand: manifest fields come from
// manifestv2, release fields from releases.json, membership from bundles.json,
// tier_pair from the other tier's registry. Base URLs are the served routes.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

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

// report collects what a run found: problems fail it, notes are printed.
type report struct{ problems, notes []string }

func (r *report) problem(format string, a ...any) {
	r.problems = append(r.problems, fmt.Sprintf(format, a...))
}
func (r *report) note(format string, a ...any) { r.notes = append(r.notes, fmt.Sprintf(format, a...)) }

// buildRegistry projects the manifests of one tier. A manifest with no
// releases.json row has no checksum and is left out with a note, or is a problem
// under requireReleased.
func buildRegistry(t tierInfo, ms map[string]*manifestv2.Manifest, rel *model.Releases, b *model.Bundles,
	peer map[string]bool, from model.GeneratedFrom, requireReleased bool) (*model.Registry, *report) {
	reg := &model.Registry{
		Generated: model.Generated, GeneratedFrom: from, SchemaVersion: model.RegistryVersion,
		Tier: t.wire, ChecksumAlgorithm: "sha256", Plugins: map[string]model.RegistryEntry{},
	}
	rep := &report{}
	for _, s := range sortedKeys(ms) {
		m := ms[s]
		if m.License != t.license {
			rep.problem("%s: license %q does not belong in the %s tree (a plugin never moves tier here)", s, m.License, t.catalog)
			continue
		}
		r, ok := rel.Releases[s]
		switch {
		case !ok && requireReleased:
			rep.problem("%s: no row in releases.json (-require-released)", s)
			continue
		case !ok:
			rep.note("%s has no row in releases.json; left out of the registry", s)
			continue
		}
		if strings.TrimPrefix(m.Version, "v") != strings.TrimPrefix(r.Version, "v") {
			rep.note("%s: manifest version %s differs from released version %s; the registry carries the release", s, m.Version, r.Version)
		}
		link, prob := linking(m, r)
		if prob != "" {
			rep.problems = append(rep.problems, prob)
			continue
		}
		reg.Plugins[s] = entryOf(t, m, r, link, membership(b, s, t.catalog), peer[s])
	}
	reg.PluginsCount = len(reg.Plugins)
	return reg, rep
}

// entryOf projects one manifest and its release row.
func entryOf(t tierInfo, m *manifestv2.Manifest, r model.Release, link linkKeys, bundles []string, pair bool) model.RegistryEntry {
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
		CLICommands: link.cliCommands,
	}
	e.Port = m.Port
	if e.Port == 0 && m.Service != nil && m.Service.Port != nil {
		e.Port = *m.Service.Port
	}
	if m.Installable != nil && !*m.Installable {
		no := false
		e.Installable = &no
	}
	if m.Runtime != "" || m.EntryPoint != "" || link.pluginType != "" || link.binaryName != "" {
		e.Implementation = &model.Implementation{Language: m.Language, Runtime: m.Runtime,
			EntryPoint: m.EntryPoint, PluginType: link.pluginType, BinaryName: link.binaryName}
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
