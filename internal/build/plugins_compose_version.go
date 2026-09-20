package build

// Purpose: strips the obsolete top-level Compose `version:` key from a
// plugin compose fragment.
// Inputs: a compose fragment's bytes.
// Outputs: the (possibly rewritten) compose fragment bytes.
// Constraints: `version:` is only ever valid (and only ever authored) as the
// very first line of a fragment — a match anywhere else is coincidental and
// must not be touched. Docker Compose has treated the top-level `version:`
// attribute as obsolete since the Compose Specification merge, and warns on
// every `nself build`/`nself start` for any plugin fragment that still
// declares one (observed: browser, google). This rewrite is idempotent: a
// fragment with no version: line is returned unchanged, and re-running
// after the first strip is a no-op.

import (
	"bytes"
	"regexp"
)

// obsoleteComposeVersionRE matches the deprecated top-level Compose
// `version:` key (only ever valid as the very first line of a fragment) plus
// one immediately following blank line, so removing it doesn't leave a
// leading blank line ahead of `services:`.
var obsoleteComposeVersionRE = regexp.MustCompile(`(?m)^version:.*\n\n?`)

// normalizeComposeDropObsoleteVersion strips a leading top-level
// `version: "3.8"` (or similar) key from a plugin compose fragment.
func normalizeComposeDropObsoleteVersion(content []byte) []byte {
	loc := obsoleteComposeVersionRE.FindIndex(content)
	if loc == nil || loc[0] != 0 {
		// version: is only ever valid (and only ever authored) as the very
		// first line of the fragment; a match anywhere else is coincidental
		// and must not be touched.
		return content
	}
	out := make([]byte, 0, len(content)-(loc[1]-loc[0]))
	out = append(out, content[loc[1]:]...)
	if bytes.Equal(out, content) {
		return content
	}
	return out
}
