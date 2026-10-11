package build

// Purpose: picks the one service of a plugin compose fragment that gets the
// core env vars (plugins_core_env.go), and the byte range that service owns.
// Inputs: the fragment bytes, the plugin directory and the plugin name.
// Outputs: a [start,end) byte range, or a "none matched" verdict.
// Constraints: a fragment can hold sidecars (forgejo ships forgejo,
// forgejo-init and forgejo-runner), so the first environment: or networks:
// block in the file is not necessarily the plugin's. The real conventions for
// the plugin's own service key are <name>, nself-<name> and plugin-<name>; the
// manifest has no service-name field. A fragment that does not parse, or has
// no services mapping, keeps the whole-content behaviour.

import (
	"bytes"

	"gopkg.in/yaml.v3"
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
// found is false when the fragment is multi-service and none of the services
// is the plugin's (the caller injects nothing). When the fragment does not
// parse or has no usable services mapping, the whole content is returned.
func pluginServiceRange(content []byte, pluginName, manifestName string) (start, end int, found bool) {
	whole := func() (int, int, bool) { return 0, len(content), true }

	var doc yaml.Node
	if yaml.Unmarshal(content, &doc) != nil || len(doc.Content) == 0 {
		return whole()
	}
	top := doc.Content[0]
	if top.Kind != yaml.MappingNode {
		return whole()
	}
	topIdx := -1
	for i := 0; i+1 < len(top.Content); i += 2 {
		if top.Content[i].Value == "services" {
			topIdx = i
			break
		}
	}
	if topIdx < 0 || top.Content[topIdx+1].Kind != yaml.MappingNode || len(top.Content[topIdx+1].Content) == 0 {
		return whole()
	}
	servicesKey, services := top.Content[topIdx], top.Content[topIdx+1]

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
			return 0, 0, false
		}
		pick = 0
	}

	offs := lineOffsets(content)
	lineStart := func(line int) int {
		if line-1 >= len(offs) {
			return len(content)
		}
		return offs[line-1]
	}
	key := services.Content[pick]
	start = lineStart(key.Line)
	switch {
	case pick+2 < len(services.Content):
		end = lineStart(services.Content[pick+2].Line)
	case topIdx+2 < len(top.Content):
		end = lineStart(top.Content[topIdx+2].Line)
	default:
		end = len(content)
	}
	// A flow-style services mapping puts keys on the services line: no safe
	// line range, keep the whole-content behaviour.
	if key.Line <= servicesKey.Line || end <= start || end > len(content) || !bytes.HasSuffix(content[:start], []byte("\n")) {
		return whole()
	}
	return start, end, true
}
