package main

// sources.go — the authored side of the image lock.
//
// Purpose: load internal/compose/images.yaml, and merge the plugins repo's
// images.json v1 (PLUG D12a) into the same Source list.
// Inputs: images.yaml bytes; images.json bytes.
// Outputs: []Source sorted by name.
// Constraints: strict YAML decoding (an unknown key is an error); a duplicate
// name is an error; images.json `ci` entries are ignored (the plugins probe
// covers them); a plugin or upstream image must carry @sha256:<index digest>.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"

	"gopkg.in/yaml.v3"
)

// Source is one authored image entry (or one merged from images.json).
type Source struct {
	Name       string   `yaml:"name"`
	Role       string   `yaml:"role"`
	Repository string   `yaml:"repository"`
	Version    string   `yaml:"version"`
	LegacyRef  string   `yaml:"legacy_ref"`
	License    string   `yaml:"license"`
	Source     string   `yaml:"source"`
	Platforms  []string `yaml:"platforms"`
	// Digest is the index digest already pinned by images.json; empty for
	// images.yaml entries (resolved from the registry).
	Digest string `yaml:"-"`
}

// LoadSources parses images.yaml.
func LoadSources(data []byte) ([]Source, error) {
	var doc struct {
		SchemaVersion int      `yaml:"schema_version"`
		Images        []Source `yaml:"images"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("images.yaml: %w", err)
	}
	if doc.SchemaVersion != 1 {
		return nil, fmt.Errorf("images.yaml: schema_version %d, want 1", doc.SchemaVersion)
	}
	seen := map[string]bool{}
	for _, s := range doc.Images {
		if s.Name == "" || s.Repository == "" || s.Version == "" || s.LegacyRef == "" || s.Role == "" {
			return nil, fmt.Errorf("images.yaml: entry %q needs name, role, repository, version and legacy_ref", s.Name)
		}
		if seen[s.Name] {
			return nil, fmt.Errorf("images.yaml: duplicate entry %q", s.Name)
		}
		seen[s.Name] = true
	}
	sort.Slice(doc.Images, func(i, j int) bool { return doc.Images[i].Name < doc.Images[j].Name })
	return doc.Images, nil
}

var pinnedRE = regexp.MustCompile(`^(.+):([^:@/]+)@(sha256:[0-9a-f]{64})$`)

// MergePlugins adds the images.json entries to base: kind plugin becomes
// `plugin/<slug>` (role plugin), kind upstream keeps its name (role optional),
// kind ci is ignored.
func MergePlugins(base []Source, data []byte) ([]Source, error) {
	var doc struct {
		Schema string `json:"schema"`
		Images map[string]struct {
			Kind      string   `json:"kind"`
			Image     string   `json:"image"`
			Platforms []string `json:"platforms"`
		} `json:"images"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("images.json: %w", err)
	}
	if doc.Schema != "nself.plugins.images/v1" {
		return nil, fmt.Errorf("images.json: schema %q, want nself.plugins.images/v1", doc.Schema)
	}
	have := map[string]bool{}
	for _, s := range base {
		have[s.Name] = true
	}
	out := append([]Source(nil), base...)
	for key, e := range doc.Images {
		var name, role string
		switch e.Kind {
		case "plugin":
			name, role = "plugin/"+key, "plugin"
		case "upstream":
			name, role = key, "optional"
		case "ci":
			continue
		default:
			return nil, fmt.Errorf("images.json: %s: kind %q is not plugin, upstream or ci", key, e.Kind)
		}
		m := pinnedRE.FindStringSubmatch(e.Image)
		if m == nil {
			return nil, fmt.Errorf("images.json: %s: image %q must be <repository>:<version>@sha256:<64 hex>", key, e.Image)
		}
		if have[name] {
			return nil, fmt.Errorf("images.json: %s duplicates an existing lock entry", name)
		}
		have[name] = true
		out = append(out, Source{Name: name, Role: role, Repository: m[1], Version: m[2], LegacyRef: m[1] + ":" + m[2],
			License: "see upstream", Source: "plugins images.json", Platforms: e.Platforms, Digest: m[3]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
