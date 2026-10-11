package reconcile

// unified_mask.go — the second pass of the non-env diff redaction (D-0321):
// hide every literal copy of the collected secret values in the lines.
//
// Inputs: the lines and the collected values. Outputs: the lines with each
// maximal run of text covered by any value replaced by one [REDACTED].
// Constraints: values may overlap ("link" and "ink-supersecret123" in
// "link-supersecret123"), so each start position takes the longest value that
// begins there and overlapping or touching matches merge into one run. Cost is
// O(line length x longest value) per line plus one trie build per call.

import (
	"sort"
	"strings"
)

// valueTrie is a byte trie of the spellings to hide: edges maps
// node<<8|byte to the child node, end marks nodes that finish a spelling.
type valueTrie struct {
	edges map[uint32]uint32
	end   []bool
}

// newValueTrie builds the trie of values and their YAML spellings (a single
// quote doubled; backslash and double quote escaped).
func newValueTrie(vals []string) *valueTrie {
	t := &valueTrie{edges: map[uint32]uint32{}, end: []bool{false}}
	jsonEsc := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	for _, v := range vals {
		for _, s := range []string{v, strings.ReplaceAll(v, "'", "''"), jsonEsc.Replace(v)} {
			t.add(s)
		}
	}
	return t
}

// add inserts one spelling.
func (t *valueTrie) add(s string) {
	node := uint32(0)
	for i := 0; i < len(s); i++ {
		key := node<<8 | uint32(s[i])
		next, ok := t.edges[key]
		if !ok {
			next = uint32(len(t.end))
			t.end = append(t.end, false)
			t.edges[key] = next
		}
		node = next
	}
	if node != 0 {
		t.end[node] = true
	}
}

// longestAt returns the length of the longest spelling that starts at s[i], or 0.
func (t *valueTrie) longestAt(s string, i int) int {
	node, best := uint32(0), 0
	for j := i; j < len(s); j++ {
		next, ok := t.edges[node<<8|uint32(s[j])]
		if !ok {
			break
		}
		node = next
		if t.end[node] {
			best = j - i + 1
		}
	}
	return best
}

// mask replaces each maximal covered run of s with one redactedValue.
func (t *valueTrie) mask(s string) string {
	var sb strings.Builder
	last, runStart, runEnd := 0, -1, 0
	for i := 0; i < len(s); i++ {
		n := t.longestAt(s, i)
		if n == 0 {
			continue
		}
		if runStart >= 0 && i <= runEnd {
			runEnd = max(runEnd, i+n)
			continue
		}
		if runStart >= 0 {
			sb.WriteString(s[last:runStart])
			sb.WriteString(redactedValue)
			last = runEnd
		}
		runStart, runEnd = i, i+n
	}
	if runStart < 0 {
		return s
	}
	sb.WriteString(s[last:runStart])
	sb.WriteString(redactedValue)
	sb.WriteString(s[runEnd:])
	return sb.String()
}

// applySecretValues is the second pass: it hides every literal copy of the
// collected values (and their YAML-quoted spellings) in lines. Text already
// redacted is left alone: each line is split on [REDACTED] first.
func applySecretValues(lines, vals []string) []string {
	if len(vals) == 0 {
		return lines
	}
	uniq := append([]string(nil), vals...)
	sort.Strings(uniq)
	t := newValueTrie(uniq)
	out := make([]string, len(lines))
	for i, line := range lines {
		parts := strings.Split(line, redactedValue)
		for j, p := range parts {
			parts[j] = t.mask(p)
		}
		out[i] = strings.Join(parts, redactedValue)
	}
	return out
}
