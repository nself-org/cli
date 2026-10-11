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
	var resolved struct {
		Services map[string]any `yaml:"services"`
	}
	if yaml.Unmarshal(content, &resolved) != nil || len(resolved.Services) == 0 {
		return 0, len(content), rangeWhole
	}
	if documentCount(content) > 1 {
		return 0, 0, rangeUnsafe
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
	if yamlGet(svc, "<<") != nil && yamlGet(svc, "environment") == nil {
		return 0, 0, rangeUnsafe // an inherited environment would be replaced
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
	return start, end, rangeFound
}
