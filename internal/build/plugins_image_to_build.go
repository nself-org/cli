package build

// Purpose: normalizes installed plugin docker-compose fragments so a plugin
// service that names an `image: nself/...` we do not publish is rebuilt from
// the plugin's own Dockerfile instead, and drops the obsolete Compose
// `version:` key that recent Docker emits a deprecation warning for.
// Inputs: a compose fragment's bytes plus the plugin directory/name pair
// already threaded through DiscoverPluginComposeFiles (plugins.go).
// Outputs: the (possibly rewritten) compose fragment bytes.
// Constraints: nself/nself-* container images (e.g. nself/nself-notify,
// nself/nself-google, nself/nself-ai, nself/nself-claw) are never published
// to Docker Hub — verified 404 on every one except nself/nself-admin — so
// any plugin compose fragment that only declares `image:` for one of them
// can never start: `nself start` fails with "pull access denied ...
// repository does not exist". Self-hosters (and CI, and every dev machine)
// must be able to build the plugin from source offline instead. This mirrors
// normalizeComposeDockerfile's precedent: cheap, targeted, regex/line-based
// text rewrites in place, not a full YAML round-trip (which would drop the
// hand-authored comments and reflow formatting in every plugin fragment).
// A service that already declares `build:` (e.g. google, browser, ai) is
// left completely untouched — it already builds from source and may keep
// `image:` alongside `build:` purely as the resulting local tag name.
// Both rewrites are idempotent: re-running on already-normalized bytes is a
// byte-for-byte no-op.

import (
	"bytes"
	"regexp"
	"strings"
)

// composeServiceLineRE matches a top-level (2-space indent) service key,
// e.g. "  notify:" — the start of a new service block within `services:`.
var composeServiceLineRE = regexp.MustCompile(`^  \S.*:\s*$`)

// composeImageNselfRE matches a service-level `image: nself/...` scalar and
// captures its leading indentation. Only images under the `nself/` registry
// namespace are ever candidates for the build: rewrite — third-party images
// (postgres, redis, meilisearch, ...) never match and are left alone.
var composeImageNselfRE = regexp.MustCompile(`^(\s*)image:\s*nself/\S+\s*$`)

// obsoleteComposeVersionRE matches the deprecated top-level Compose
// `version:` key (only ever valid as the very first line of a fragment) plus
// one immediately following blank line, so removing it doesn't leave a
// leading blank line ahead of `services:`.
var obsoleteComposeVersionRE = regexp.MustCompile(`(?m)^version:.*\n\n?`)

// normalizeComposeImageToBuild rewrites a plugin compose fragment so any
// service-level `image: nself/<name>...` line becomes a `build:` block that
// compiles the image from the plugin's own Dockerfile at
// ${NSELF_PLUGIN_DIR}/<pluginName>, when (a) that Dockerfile actually exists
// and (b) the enclosing service does not already declare `build:` (in which
// case it already builds from source and is left untouched, image: and all).
func normalizeComposeImageToBuild(content []byte, pluginDir, pluginName string) []byte {
	canonical := canonicalDockerfile(pluginDir, pluginName)
	if canonical == "" {
		return content // no Dockerfile to build from — nothing safe to do
	}

	original := string(content)
	lines := strings.Split(original, "\n")
	changed := false

	for i, line := range lines {
		m := composeImageNselfRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		indent := m[1]

		// Find the enclosing service block: walk back to its "  name:"
		// header line, then forward to the line before the next sibling at
		// the same or a shallower indent (next service, or a new top-level
		// key such as networks:/volumes:).
		blockStart := 0
		for j := i - 1; j >= 0; j-- {
			if composeServiceLineRE.MatchString(lines[j]) {
				blockStart = j
				break
			}
		}
		blockEnd := len(lines)
		for j := blockStart + 1; j < len(lines); j++ {
			if j == i {
				continue
			}
			trimmed := strings.TrimRight(lines[j], " \t")
			if trimmed == "" {
				continue
			}
			leading := len(lines[j]) - len(strings.TrimLeft(lines[j], " \t"))
			if leading <= 2 {
				blockEnd = j
				break
			}
		}

		// Already builds from source? Leave the whole service alone.
		hasBuild := false
		for j := blockStart; j < blockEnd; j++ {
			if strings.HasPrefix(lines[j], indent+"build:") {
				hasBuild = true
				break
			}
		}
		if hasBuild {
			continue
		}

		lines[i] = indent + "build:\n" +
			indent + "  context: ${NSELF_PLUGIN_DIR}/" + pluginName + "\n" +
			indent + "  dockerfile: " + canonical
		changed = true
	}

	if !changed {
		return content
	}
	return []byte(strings.Join(lines, "\n"))
}

// normalizeComposeDropObsoleteVersion strips a leading top-level
// `version: "3.8"` (or similar) key from a plugin compose fragment. Docker
// Compose has treated the top-level `version:` attribute as obsolete since
// the Compose Specification merge, and warns on every `nself build`/`nself
// start` for any plugin fragment that still declares one (observed: browser,
// google). Idempotent — a fragment with no version: line is returned
// unchanged, and re-running after the first strip is a no-op.
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
