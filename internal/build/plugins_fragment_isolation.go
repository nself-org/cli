package build

// Purpose: the fragment rules that keep a plugin's compose fragment inside its
// own stack, beside the network rules of plugins_network_resolved.go (D-0490).
// Inputs: the resolved fragment (aliases and merge keys expanded).
// Outputs: one violation string per breach, sorted; empty when the fragment
// keeps to itself.
// Constraints: each rule is a fact about the decoded fragment, never about
// the bytes, so a merge key or an alias cannot hide a breach. A breach is an
// E129 in v1.5 and a warning in v1.4 (plugins_network_normalize.go).

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/compose"
)

// isolationViolations lists what the fragment does to reach outside itself:
//   - include: pulls another compose file in, past every rule here;
//   - extends with a file: inherits a service of another file, including its
//     network_mode;
//   - build.network other than none or default: the image build joins the
//     host network or one by name;
//   - a service listing a network its own fragment does not declare and that
//     is not the project network: after the merge it joins the network of
//     another plugin;
//   - a service named like a core service: it merges into that service and
//     is given the plugin core env.
func (r *resolvedFragment) isolationViolations() []string {
	var out []string
	if r.Include != nil {
		out = append(out, "top-level include")
	}
	declared := map[string]bool{}
	for key := range r.Networks {
		declared[key] = true
	}
	core := coreServiceNames()
	for name, s := range r.Services {
		if core[name] {
			out = append(out, fmt.Sprintf("%s: defines the core service %s", name, name))
		}
		if s.hasExtendsFile() {
			out = append(out, fmt.Sprintf("%s: extends another file", name))
		}
		if mode := s.buildNetwork(); mode != "" && mode != "none" && mode != "default" {
			out = append(out, fmt.Sprintf("%s: build.network %s", name, mode))
		}
		for _, key := range s.networkKeys() {
			if !declared[key] && !isProjectNetworkKey(key) {
				out = append(out, fmt.Sprintf("%s: network %s is not declared by this fragment", name, key))
			}
		}
	}
	sort.Strings(out)
	return out
}

// coreServiceNames is the set of services every stack carries; a plugin
// fragment may not define one.
func coreServiceNames() map[string]bool {
	names := map[string]bool{}
	for _, e := range compose.CoreServices() {
		names[e.Name] = true
	}
	return names
}

// isProjectNetworkKey reports whether key names the project network (or
// compose's own per-project default), which every service may join.
func isProjectNetworkKey(key string) bool {
	return key == "default" || projectNetworkRE.MatchString(key) || isLegacyNetwork(key)
}

// hasExtendsFile reports whether the service extends a service of another
// file. A short or unreadable extends is treated as one (fail closed); an
// extends of a service in the same file inherits nothing the fragment's own
// checks do not already see.
func (s resolvedService) hasExtendsFile() bool {
	switch v := s.Extends.(type) {
	case nil:
		return false
	case map[string]any:
		file, ok := v["file"]
		return ok && file != nil
	}
	return true
}

// buildNetwork returns the lower-cased build.network, "" when the service
// builds no image or sets none. A non-string value comes back as its text.
func (s resolvedService) buildNetwork() string {
	b, ok := s.Build.(map[string]any)
	if !ok || b["network"] == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(fmt.Sprint(b["network"])))
}

// networkKeys returns the network keys the service lists, in list or mapping
// form.
func (s resolvedService) networkKeys() []string {
	var out []string
	switch v := s.Networks.(type) {
	case []any:
		for _, e := range v {
			if name, ok := e.(string); ok {
				out = append(out, name)
			}
		}
	case map[string]any:
		for name := range v {
			out = append(out, name)
		}
	}
	return out
}
