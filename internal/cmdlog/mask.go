package cmdlog

// Purpose: keep secret values out of the command log (D-0473).
// Inputs: secret values from the caller (Begin) and from MaskValues.
// Outputs: scrubbed argv elements and error text.
// Constraints: the log holds argv plus the error text only, never command
// output, so a --reveal result cannot reach it. A value of 4 or more bytes is
// replaced wherever it appears; a shorter one only when it is a whole argv
// element, the whole text after "--x=", or the whole error. Empty values are
// ignored.

import (
	"sort"
	"strings"
	"sync"
)

const redactedText = "[REDACTED]"

// minSubstringLen is the shortest secret replaced inside a longer string.
const minSubstringLen = 4

var (
	maskMu     sync.Mutex
	maskValues []string
)

// MaskValues registers secret values every later Begin and finish scrubs from
// the log. Safe for concurrent use; empty values are skipped.
func MaskValues(vals ...string) {
	maskMu.Lock()
	defer maskMu.Unlock()
	for _, v := range vals {
		if v != "" {
			maskValues = append(maskValues, v)
		}
	}
}

// secretSet merges the package-level values with extra, longest first, no
// empties and no duplicates.
func secretSet(extra []string) []string {
	maskMu.Lock()
	all := append(append([]string{}, maskValues...), extra...)
	maskMu.Unlock()
	seen := make(map[string]bool, len(all))
	out := all[:0]
	for _, v := range all {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

// scrubText replaces secrets inside s (substring for long values, whole-string
// for short ones).
func scrubText(s string, secrets []string) string {
	for _, v := range secrets {
		switch {
		case len(v) >= minSubstringLen:
			s = strings.ReplaceAll(s, v, redactedText)
		case s == v:
			s = redactedText
		}
	}
	return s
}

// scrubElem scrubs one argv element, including the value of a "--x=value".
func scrubElem(arg string, secrets []string) string {
	arg = scrubText(arg, secrets)
	if strings.HasPrefix(arg, "-") {
		if name, val, ok := strings.Cut(arg, "="); ok {
			return name + "=" + scrubText(val, secrets)
		}
	}
	return arg
}
