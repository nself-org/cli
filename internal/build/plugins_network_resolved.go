package build

// Purpose: the resolved view of a plugin compose fragment's networks, used for
// every policy decision (foreign external network, which services still need a
// network attached, whether the legacy nself_network is left).
// Inputs: the fragment bytes.
// Outputs: Go values decoded by yaml.v3, so aliases and `<<` merge keys are
// expanded the way compose expands them.
// Constraints: decisions read this view; the yaml.Node tree is kept only to
// plan byte-splice edits (plugins_network_normalize.go), because a node walk
// misses a network or service that an alias or merge key supplies.

import (
	"sort"

	"gopkg.in/yaml.v3"
)

// resolvedNetwork is one top-level network definition.
type resolvedNetwork struct {
	External any    `yaml:"external"`
	Name     string `yaml:"name"`
}

// resolvedService is the part of a service the network pass looks at.
type resolvedService struct {
	Networks    any `yaml:"networks"`
	NetworkMode any `yaml:"network_mode"`
}

// resolvedFragment is a fragment with aliases and merge keys expanded.
type resolvedFragment struct {
	Services map[string]resolvedService `yaml:"services"`
	Networks map[string]resolvedNetwork `yaml:"networks"`
}

// resolveFragment decodes content; an error means it cannot be decoded.
func resolveFragment(content []byte) (*resolvedFragment, error) {
	var r resolvedFragment
	if err := yaml.Unmarshal(content, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// isExternal reports whether the network is external: `external: true` (any
// spelling yaml.v3 decodes as true) or an `external:` mapping.
func (n resolvedNetwork) isExternal() bool {
	switch v := n.External.(type) {
	case bool:
		return v
	case map[string]any:
		return true
	}
	return false
}

// realName is the name compose uses: name:, else external.name, else the key.
func (n resolvedNetwork) realName(key string) string {
	if n.Name != "" {
		return n.Name
	}
	if m, ok := n.External.(map[string]any); ok {
		if s, ok := m["name"].(string); ok && s != "" {
			return s
		}
	}
	return key
}

// foreignExternalNetworks returns the sorted names of top-level networks the
// fragment declares external that are not the project network.
func (r *resolvedFragment) foreignExternalNetworks() []string {
	var out []string
	for key, n := range r.Networks {
		if !n.isExternal() {
			continue
		}
		if name := n.realName(key); !projectNetworkRE.MatchString(name) && name != legacyProjectNetwork {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// legacyKeys returns the keys of external networks that are the legacy
// literal: key `nself_network` with no name, or a name that is it.
func (r *resolvedFragment) legacyKeys() map[string]bool {
	keys := map[string]bool{}
	for key, n := range r.Networks {
		if n.isExternal() && n.realName(key) == legacyProjectNetwork {
			keys[key] = true
		}
	}
	return keys
}

// usesNetworks reports whether the service has a networks: or network_mode:
// of its own (written or merged in), so no network is attached to it.
func (s resolvedService) usesNetworks() bool { return s.Networks != nil || s.NetworkMode != nil }

// refersTo reports whether the service lists any of keys as a network, in
// list or mapping form.
func (s resolvedService) refersTo(keys map[string]bool) bool {
	switch v := s.Networks.(type) {
	case []any:
		for _, e := range v {
			if name, ok := e.(string); ok && keys[name] {
				return true
			}
		}
	case map[string]any:
		for name := range v {
			if keys[name] {
				return true
			}
		}
	}
	return false
}
