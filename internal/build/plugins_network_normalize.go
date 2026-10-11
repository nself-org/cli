package build

// Purpose: puts every service of an installed plugin's compose fragment on the
// project network and refuses fragments that reach for another external one.
// Inputs: the fragment bytes and the plugin's short name.
// Outputs: the fragment bytes with missing networks attached and
// ${DOCKER_NETWORK:-x} written as ${DOCKER_NETWORK}, or an E129 error.
// Constraints: byte-splice edits at yaml.v3 node lines, so comments, quoting
// and layout survive (same constraint as plugins_core_env_inject.go). v1.5
// only: v1.4 never rewrites a fragment and only warns about a foreign
// external network. A service with network_mode keeps it untouched.

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
)

// dockerNetworkDefaultRE matches ${DOCKER_NETWORK:-<fallback>} as a whole value.
var dockerNetworkDefaultRE = regexp.MustCompile(`^\$\{DOCKER_NETWORK:-[^}]*\}$`)

// projectNetworkRE matches the names that are the project network: the
// DOCKER_NETWORK variable or <project>_network spelled with the project vars.
// The legacy literal legacyProjectNetwork is the project network too
// (plugins_network_legacy.go) and is checked beside this pattern.
var projectNetworkRE = regexp.MustCompile(`^\$\{DOCKER_NETWORK(:-[^}]*)?\}$|^\$\{(COMPOSE_PROJECT_NAME|PROJECT_NAME)(:-[^}]*)?\}_network$`)

// lineEdit is one splice: replace old with new on a 1-based line, or (old
// empty) insert the lines of new before it. A col > 0 (1-based) replaces old
// only at that column; col 0 replaces its first occurrence on the line. A
// drop > 0 deletes that many whole lines starting at the line.
type lineEdit struct {
	line, col, drop int
	old, new        string
}

// yamlGet returns the value node for key in mapping m, or nil.
func yamlGet(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// serviceNetworkEdits plans the edits for one service mapping.
// res is the service's resolved view: a service whose networks or network_mode
// arrive through a merge key must not get a second networks: key (it would
// override the merged one).
func serviceNetworkEdits(svc *yaml.Node, res resolvedService, cr string) []lineEdit {
	if svc.Kind != yaml.MappingNode || svc.Style&yaml.FlowStyle != 0 || len(svc.Content) == 0 {
		return nil
	}
	if yamlGet(svc, "network_mode") != nil {
		return nil
	}
	nets := yamlGet(svc, "networks")
	if nets == nil && res.usesNetworks() {
		return nil
	}
	if nets == nil {
		first := svc.Content[0]
		ind := strings.Repeat(" ", first.Column-1)
		return []lineEdit{{line: first.Line, new: ind + "networks:" + cr + "\n" + ind + "  - ${DOCKER_NETWORK}" + cr + "\n"}}
	}
	var edits []lineEdit
	for i, n := range nets.Content {
		if nets.Kind == yaml.MappingNode && i%2 == 1 {
			continue // a network's own settings, not its name
		}
		if n.Kind == yaml.ScalarNode && dockerNetworkDefaultRE.MatchString(n.Value) {
			edits = append(edits, lineEdit{line: n.Line, old: n.Value, new: "${DOCKER_NETWORK}"})
		}
	}
	return edits
}

// normalizeComposeNetworks attaches missing networks to ${DOCKER_NETWORK},
// rewrites ${DOCKER_NETWORK:-x}, and rejects a foreign external network
// (E129; a warning in v1.4 mode, which also rewrites nothing).
func normalizeComposeNetworks(content []byte, pluginName string) ([]byte, error) {
	var doc yaml.Node
	if yaml.Unmarshal(content, &doc) != nil || len(doc.Content) == 0 {
		return content, nil // unparseable: other normalizers and compose report it
	}
	top := doc.Content[0]
	res, err := resolveFragment(content)
	if err != nil {
		return content, nil // decodes as nodes but not as values: compose reports it
	}
	if foreign := res.foreignExternalNetworks(); len(foreign) > 0 {
		msg := fmt.Sprintf("plugin %q attaches external network %s other than the project network", pluginName, strings.Join(foreign, ", "))
		// compat.V15(P7-PLUG-17): a fragment may join any external network (warning) -> E129 fails the build
		if compat.V15() {
			return content, errs.New("E129", msg)
		}
		warnOnce(msg)
	}
	// compat.V15(P7-PLUG-17): fragments keep their own networks and ${DOCKER_NETWORK:-x} -> missing networks attached, ${DOCKER_NETWORK:-x} written as ${DOCKER_NETWORK}
	if !compat.V15() {
		return content, nil
	}
	cr := ""
	if bytes.Contains(content, []byte("\r\n")) {
		cr = "\r"
	}
	var edits []lineEdit
	if svcs := yamlGet(top, "services"); svcs != nil && svcs.Kind == yaml.MappingNode {
		for i := 1; i < len(svcs.Content); i += 2 {
			edits = append(edits, serviceNetworkEdits(svcs.Content[i], res.Services[svcs.Content[i-1].Value], cr)...)
		}
	}
	legacy, keys, why := legacyNetworkEdits(content, top, res)
	if why != "" {
		return content, errs.New("E129", fmt.Sprintf("plugin %q uses the legacy external network %s in a form the build cannot rewrite to ${DOCKER_NETWORK} (%s)", pluginName, legacyProjectNetwork, why))
	}
	out := applyLineEdits(content, append(edits, legacy...))
	if len(keys) > 0 && legacyNetworkRemains(out, keys) {
		return content, errs.New("E129", fmt.Sprintf("plugin %q uses the legacy external network %s in a form the build cannot rewrite to ${DOCKER_NETWORK} (a definition or reference is supplied through an alias or merge key)", pluginName, legacyProjectNetwork))
	}
	return out, nil
}

// applyLineEdits applies edits bottom-up so earlier line numbers stay valid.
func applyLineEdits(content []byte, edits []lineEdit) []byte {
	if len(edits) == 0 {
		return content
	}
	sort.SliceStable(edits, func(a, b int) bool {
		if edits[a].line != edits[b].line {
			return edits[a].line > edits[b].line
		}
		return edits[a].col > edits[b].col
	})
	lines := strings.Split(string(content), "\n")
	for _, e := range edits {
		if e.line < 1 || e.line > len(lines) {
			continue
		}
		if e.drop > 0 {
			end := e.line - 1 + e.drop
			if end > len(lines) {
				end = len(lines)
			}
			lines = append(lines[:e.line-1], lines[end:]...)
		} else if e.old == "" {
			lines[e.line-1] = e.new + lines[e.line-1]
		} else if e.col > 0 {
			if l := lines[e.line-1]; e.col-1 <= len(l) && strings.HasPrefix(l[e.col-1:], e.old) {
				lines[e.line-1] = l[:e.col-1] + e.new + l[e.col-1+len(e.old):]
			}
		} else {
			lines[e.line-1] = strings.Replace(lines[e.line-1], e.old, e.new, 1)
		}
	}
	return []byte(strings.Join(lines, "\n"))
}
