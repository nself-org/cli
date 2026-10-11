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
// The legacy literals (legacyNetworkNames, plugins_network_legacy.go) are the
// project network too and are checked beside this pattern.
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

// policyError is an E129 that states its own cause: the registered text
// describes a foreign external network, which is only one of the cases.
func policyError(what, why, fix string) *errs.CLIError {
	e := errs.New("E129", what)
	e.Why, e.Fix = why, fix
	return e
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
	if documentCount(content) > 1 {
		msg := fmt.Sprintf("plugin %q has a compose fragment with more than one YAML document; the network policy checks the first only", pluginName)
		// compat.V15(P7-PLUG-17): a fragment with several YAML documents is checked on the first only (warning) -> E129, the fragment must be one document
		if compat.V15() {
			return content, policyError(msg, "Compose merges every document of a file, so a network declared after --- would escape the network policy.", "Put the whole fragment in one YAML document (remove ---), or ask the plugin author to.")
		}
		warnOnce(msg)
	}
	res, err := resolveFragment(content)
	if err != nil {
		// compat.V15(P7-PLUG-17): a fragment that decodes as nodes but not as values is passed on unchecked -> E129, the network policy could not be checked
		if compat.V15() {
			return content, policyError(fmt.Sprintf("plugin %q: the network policy could not be checked, the compose fragment does not decode (%v)", pluginName, err),
				"A network, name or service field has a shape compose may read differently from the policy check.", "Fix the fragment's networks:, services: and network_mode: entries, or ask the plugin author to.")
		}
		return content, nil
	}
	if foreign := res.foreignExternalNetworks(pluginName); len(foreign) > 0 {
		msg := fmt.Sprintf("plugin %q attaches external network %s other than the project network", pluginName, strings.Join(foreign, ", "))
		// compat.V15(P7-PLUG-17): a fragment may join any external network (warning) -> E129 fails the build
		if compat.V15() {
			return content, policyError(msg, "The plugin's docker-compose.plugin.yml joins an external network, or a named network that is not project-scoped, other than the project network; compose reuses an existing network of that name.", "Attach the service to ${DOCKER_NETWORK} only, or remove the plugin; ask the plugin author to drop the network from the fragment.")
		}
		warnOnce(msg)
	}
	if modes := res.badNetworkModes(); len(modes) > 0 {
		msg := fmt.Sprintf("plugin %q uses network_mode other than none or service:<a service of this fragment> (%s)", pluginName, strings.Join(modes, ", "))
		// compat.V15(P7-PLUG-17): a service may set any network_mode (host, container:x, a network name) -> E129 unless it is none or service:<a service of this fragment>
		if compat.V15() {
			return content, policyError(msg, "network_mode host, container:x or a network name puts the service on a network or namespace outside this stack.", "Remove network_mode so the service joins ${DOCKER_NETWORK}, or ask the plugin author to.")
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
		// A service supplied by a merge key under services: has no node to
		// splice a networks: into, and a written key would replace it whole.
		for name, rs := range res.Services {
			if yamlGet(svcs, name) == nil && !rs.usesNetworks() {
				return content, policyError(fmt.Sprintf("plugin %q: service %q comes from a merge key under services: and has no networks; write the service out with networks: [${DOCKER_NETWORK}]", pluginName, name),
					"A service supplied by a merge key under services: has no place to attach the project network.", "Write the service out under services: with networks: [${DOCKER_NETWORK}], or ask the plugin author to.")
			}
		}
	}
	legacy, keys, why := legacyNetworkEdits(content, top, res)
	if why != "" {
		return content, policyError(fmt.Sprintf("plugin %q uses the %s in a form the build cannot rewrite to ${DOCKER_NETWORK} (%s)", pluginName, legacyNetworkLabel, why), legacyRewriteWhy, legacyRewriteFix)
	}
	out := applyLineEdits(content, append(edits, legacy...))
	if len(keys) > 0 && legacyNetworkRemains(out, keys) {
		return content, policyError(fmt.Sprintf("plugin %q uses the %s in a form the build cannot rewrite to ${DOCKER_NETWORK} (a definition or reference is supplied through an alias or merge key)", pluginName, legacyNetworkLabel), legacyRewriteWhy, legacyRewriteFix)
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
