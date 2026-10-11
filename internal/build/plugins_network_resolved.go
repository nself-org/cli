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
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

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

// isExternal reports whether the network is external, failing closed: compose
// reads `external: "true"`, `yes`, `on` and `${VAR:-true}` as external too, so
// everything is external except an absent value and a false (bool, or the
// strings false, no, n, off in any case).
func (n resolvedNetwork) isExternal() bool {
	switch v := n.External.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "false", "no", "n", "off":
			return false
		}
	}
	return true
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

// projectScopedFor reports whether name is a network of this project that
// belongs to plugin: ${COMPOSE_PROJECT_NAME[:-x]}_<plugin>_<suffix> or
// ${PROJECT_NAME[:-x]}_<plugin>_<suffix>. The plugin's own namespace is
// required: a bare ${COMPOSE_PROJECT_NAME}_x_network would land on the default
// network of a sibling project named <project>_x. Plugin names match
// ^[a-z][a-z0-9-]*$ (no underscore), so "_<plugin>_" cannot match a longer
// plugin name. An empty plugin accepts nothing (fail closed).
func projectScopedFor(plugin, name string) bool {
	if plugin == "" {
		return false
	}
	re := regexp.MustCompile(`^\$\{(COMPOSE_PROJECT_NAME|PROJECT_NAME)(:-[^}]*)?\}_` + regexp.QuoteMeta(plugin) + `_.+$`)
	return re.MatchString(name)
}

// foreignExternalNetworks returns the sorted names of top-level networks the
// fragment would join that are not the project network: an external network
// that is not the project network or a legacy literal, and a network that is
// not external but carries an explicit name that is not scoped to this plugin
// of this project (compose reuses an existing network of that name instead of
// creating one).
func (r *resolvedFragment) foreignExternalNetworks(pluginName string) []string {
	var out []string
	for key, n := range r.Networks {
		if n.isExternal() {
			if name := n.realName(key); !projectNetworkRE.MatchString(name) && !isLegacyNetwork(name) {
				out = append(out, name)
			}
			continue
		}
		if n.Name != "" && !projectNetworkRE.MatchString(n.Name) && !projectScopedFor(pluginName, n.Name) {
			out = append(out, n.Name)
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
		if n.isExternal() && isLegacyNetwork(n.realName(key)) {
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

// documentCount returns how many YAML documents content holds, counting a
// null document ("---" with nothing after it) when it comes before real
// content: "---\n---\nservices: ..." is two, because the policy decodes only
// the first (null) one and would never see the services. Null documents after
// the last real one ("services: ...\n---\n") hide nothing and are not
// counted. An empty stream, or one of only comments, counts 0. Compose merges
// every document of a -f file, while the policy decodes the first one only, so
// a fragment with more than one is refused.
func documentCount(content []byte) int {
	dec := yaml.NewDecoder(bytes.NewReader(content))
	n, upToLastReal := 0, 0
	for {
		var doc yaml.Node
		if err := dec.Decode(&doc); err != nil {
			if !errors.Is(err, io.EOF) {
				return n + 1 // a document that fails to parse is still a document
			}
			return upToLastReal
		}
		n++
		if len(doc.Content) > 0 && (doc.Content[0].Kind != yaml.ScalarNode || doc.Content[0].Tag != "!!null") {
			upToLastReal = n
		}
	}
}

// badNetworkModes returns "service: mode" for every service whose network_mode
// is not none or service:<a service defined in this fragment>. Any other value
// (host, bridge, container:x, a network name, a variable) joins a network or
// namespace outside this stack; compose accepts a network name there.
func (r *resolvedFragment) badNetworkModes() []string {
	var out []string
	for name, s := range r.Services {
		if s.NetworkMode == nil {
			continue
		}
		mode, _ := s.NetworkMode.(string)
		if mode == "none" {
			continue
		}
		if target, ok := strings.CutPrefix(mode, "service:"); ok {
			if _, defined := r.Services[target]; defined {
				continue
			}
		}
		out = append(out, fmt.Sprintf("%s: %v", name, s.NetworkMode))
	}
	sort.Strings(out)
	return out
}
