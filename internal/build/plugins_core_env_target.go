package build

// Purpose: picks the one service of a plugin compose fragment that gets the
// core env vars (plugins_core_env.go), and the byte range that service owns.
// Inputs: the fragment bytes, the plugin directory and the plugin name.
// Outputs: a [start,end) byte range, or a verdict saying why there is none.
// Constraints: a fragment can hold sidecars (forgejo ships forgejo,
// forgejo-init and forgejo-runner), so the first environment: or networks:
// block in the file is not necessarily the plugin's. The real conventions for
// the plugin's own service key are <name>, nself-<name> and plugin-<name>; the
// manifest has no service-name field. A fragment that does not parse, or has
// no services at all, keeps the whole-content behaviour. A shape this text
// range cannot be trusted on (aliases, flow style, merge keys, several
// documents) is "unsafe": the caller injects nothing and warns.

import (
	"bytes"
	"regexp"

	"gopkg.in/yaml.v3"
)

// serviceRangeState says what pluginServiceRange found.
type serviceRangeState int

const (
	rangeWhole   serviceRangeState = iota // no services anywhere: use the whole content
	rangeFound                            // the plugin's service owns [start,end)
	rangeNoMatch                          // several services, none is the plugin's
	rangeUnsafe                           // no safe range can be picked
)

// pluginServiceCandidates lists the service keys that can be the plugin's own,
// in priority order: the plugin name, then the manifest name when it differs.
func pluginServiceCandidates(pluginName, manifestName string) []string {
	names := []string{pluginName}
	if manifestName != "" && manifestName != pluginName {
		names = append(names, manifestName)
	}
	var out []string
	for _, n := range names {
		out = append(out, n, "nself-"+n, "plugin-"+n)
	}
	return out
}

// lineOffsets returns the byte offset of the start of every line (lines are
// split on \n, so \r\n files keep matching yaml.v3's 1-based line numbers).
func lineOffsets(content []byte) []int {
	offs := []int{0}
	for i, b := range content {
		if b == '\n' {
			offs = append(offs, i+1)
		}
	}
	return offs
}

// pluginServiceRange finds the byte range of the plugin's own service.
func pluginServiceRange(content []byte, pluginName, manifestName string) (start, end int, state serviceRangeState) {
	var doc yaml.Node
	if yaml.Unmarshal(content, &doc) != nil || len(doc.Content) == 0 {
		return 0, len(content), rangeWhole
	}
	// Before the services check: the decode below reads the first document
	// only, so services in a later one would look like none at all.
	if documentCount(content) > 1 {
		return 0, 0, rangeUnsafe
	}
	var resolved struct {
		Services map[string]any `yaml:"services"`
	}
	if yaml.Unmarshal(content, &resolved) != nil || len(resolved.Services) == 0 {
		return 0, len(content), rangeWhole
	}

	top := doc.Content[0]
	topIdx := -1
	if top.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(top.Content); i += 2 {
			if top.Content[i].Kind == yaml.ScalarNode && top.Content[i].Value == "services" {
				topIdx = i
				break
			}
		}
	}
	if topIdx < 0 {
		return 0, 0, rangeUnsafe // services arrives through a top-level merge key
	}
	servicesKey, services := top.Content[topIdx], top.Content[topIdx+1]
	if services.Kind != yaml.MappingNode || services.Style&yaml.FlowStyle != 0 {
		return 0, 0, rangeUnsafe // alias or flow style: no line range
	}
	for i := 0; i+1 < len(services.Content); i += 2 {
		k := services.Content[i]
		if k.Kind != yaml.ScalarNode || k.Value == "<<" || k.Line <= servicesKey.Line {
			return 0, 0, rangeUnsafe
		}
	}

	pick := -1
	for _, cand := range pluginServiceCandidates(pluginName, manifestName) {
		for i := 0; i+1 < len(services.Content); i += 2 {
			if services.Content[i].Value == cand {
				pick = i
				break
			}
		}
		if pick >= 0 {
			break
		}
	}
	if pick < 0 {
		if len(services.Content) != 2 {
			return 0, 0, rangeNoMatch
		}
		pick = 0
	}
	svc := services.Content[pick+1]
	if svc.Kind != yaml.MappingNode || svc.Style&yaml.FlowStyle != 0 {
		return 0, 0, rangeUnsafe
	}
	if svc.Anchor != "" {
		return 0, 0, rangeUnsafe // another service may inherit it and would get the env too
	}
	if yamlGet(svc, "<<") != nil && yamlGet(svc, "environment") == nil {
		return 0, 0, rangeUnsafe // an inherited environment would be replaced
	}
	if hasLoneCR(content) {
		return 0, 0, rangeUnsafe // yaml.v3 counts a lone CR as a line break, the offsets do not
	}

	offs := lineOffsets(content)
	lineStart := func(line int) int {
		if line-1 >= len(offs) {
			return len(content)
		}
		return offs[line-1]
	}
	start = lineStart(services.Content[pick].Line)
	switch {
	case pick+2 < len(services.Content):
		end = lineStart(services.Content[pick+2].Line)
	case topIdx+2 < len(top.Content):
		end = lineStart(top.Content[topIdx+2].Line)
	default:
		end = len(content)
	}
	if end <= start || end > len(content) || !bytes.HasSuffix(content[:start], []byte("\n")) {
		return 0, 0, rangeUnsafe
	}
	if !directKeysBindable(content[start:end], services.Content[pick].Line, svc) {
		return 0, 0, rangeUnsafe
	}
	return start, end, rangeFound
}

// hasLoneCR reports a '\r' that is not part of a "\r\n" pair.
func hasLoneCR(content []byte) bool {
	for i, b := range content {
		if b == '\r' && (i+1 >= len(content) || content[i+1] != '\n') {
			return true
		}
	}
	return false
}

// yamlKey returns the key node for key in mapping m, or nil.
func yamlKey(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i]
		}
	}
	return nil
}

var (
	envLineRE  = regexp.MustCompile(`^[ \t]+environment:`)
	netsLineRE = regexp.MustCompile(`^[ \t]+networks:`)
)

// directKeysBindable checks that the text rewrite will act on the service's
// DIRECT environment (or, with none, its direct short networks list), not on a
// nested key or a block scalar that merely has the same text. seg is the
// service's byte range and firstLine the 1-based file line it starts at.
// The regexes the rewrite uses take the first match in seg, so the first
// matching line must be the direct key's line.
func directKeysBindable(seg []byte, firstLine int, svc *yaml.Node) bool {
	envKey, netKey := yamlKey(svc, "environment"), yamlKey(svc, "networks")
	firstEnv, firstNets := -1, -1
	for i, line := range bytes.Split(seg, []byte("\n")) {
		if firstEnv < 0 && envLineRE.Match(line) {
			firstEnv = firstLine + i
		}
		if firstNets < 0 && netsLineRE.Match(line) {
			firstNets = firstLine + i
		}
	}
	if envKey != nil {
		return firstEnv == envKey.Line
	}
	if firstEnv >= 0 {
		return false // a nested or block-scalar "environment:" and no direct one
	}
	if netKey == nil {
		return firstNets < 0
	}
	// The short-list anchor must be the direct networks: key, in block list form.
	if nets := yamlGet(svc, "networks"); nets.Kind != yaml.SequenceNode || nets.Style&yaml.FlowStyle != 0 {
		return firstNets == netKey.Line && !hasLaterLine(seg, firstLine, netsLineRE, netKey.Line)
	}
	return firstNets == netKey.Line
}

// hasLaterLine reports a line of seg after the given file line that matches re.
func hasLaterLine(seg []byte, firstLine int, re *regexp.Regexp, after int) bool {
	for i, line := range bytes.Split(seg, []byte("\n")) {
		if firstLine+i > after && re.Match(line) {
			return true
		}
	}
	return false
}
