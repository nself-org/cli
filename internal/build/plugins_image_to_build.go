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
	"log/slog"
	"os"
	"path/filepath"
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

// composeBuildLineRE matches a service-level `build:` mapping header (any
// indentation, so it fires for both a 4-space service->build and a deeper
// nesting) and captures that indentation to find the block's extent.
var composeBuildLineRE = regexp.MustCompile(`^(\s*)build:\s*$`)

// composeBuildContextRE and composeBuildDockerfileRE match a `context:` /
// `dockerfile:` scalar line inside a build: block, capturing indentation
// (group 1, reused verbatim on rewrite so we never guess at the author's
// spacing) and the raw value (group 2).
var composeBuildContextRE = regexp.MustCompile(`^(\s*)context:\s*(\S+)\s*$`)
var composeBuildDockerfileRE = regexp.MustCompile(`^(\s*)dockerfile:\s*(\S+)\s*$`)

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

// normalizeComposeBuildContext rewrites a plugin compose fragment's
// build.context/build.dockerfile pair when the context was authored for the
// SOURCE REPO layout instead of the INSTALLED layout.
//
// WHY: a plugin's docker-compose.plugin.yml lives at
// <repo>/{free,paid}/<name>/docker-compose.plugin.yml in source but is
// installed to ~/.nself/plugins/<name>/docker-compose.plugin.yml — one path
// segment shallower, with no {free,paid}/<name> prefix at all. A build
// context authored against the source layout, e.g.
// "${NSELF_PLUGIN_DIR}/../.." + "dockerfile: free/cron/Dockerfile" (cron,
// push) or a bare "../.." + "dockerfile: paid/nself-alert-router/Dockerfile"
// (nself-alert-router), resolves two levels up from the source repo root to
// the plugin dir, then back down through free/cron — correct only in that
// exact tree. Once installed, two levels up from
// ~/.nself/plugins/cron/docker-compose.plugin.yml is $HOME, which has no
// free/ directory, so `docker compose build` fails opaquely: "resolve :
// lstat /home/<user>/free: no such file or directory" (E2E golden path step
// 13, defect #10). 31 other shipped plugins already use the correct
// installed-layout shape (`context: .`); ai/claw/mux/voice use
// `context: ${NSELF_PLUGIN_DIR}/<name>` — this rewrite converges every
// plugin onto that second shape, which is layout-independent (it names the
// plugin's own installed directory explicitly rather than climbing to it).
//
// Detection is two independent signals, either one sufficient: (a) the
// context value, after substituting ${NSELF_PLUGIN_DIR} for the plugin's own
// directory, contains a ".." path component that walks out of it, or (b) the
// dockerfile value names a directory (anything but a bare filename) — a
// correctly-scoped context never needs one, since Docker resolves dockerfile
// relative to context.
//
// The rewritten dockerfile value is resolved by resolveDockerfileName: the
// BASENAME of the originally-authored value when that exact file exists at
// the installed plugin root (e.g. "paid/nself-alert-router/Dockerfile.golang"
// -> "Dockerfile.golang" if that file ships there), else the canonical bare
// "Dockerfile" when that exists, else the plugin is left completely
// untouched and a warning names it — guessing wrong here would trade one
// silent build failure for another that is harder to diagnose (a Dockerfile
// that "exists" at the wrong path, or the wrong Dockerfile entirely when a
// plugin ships more than one). This is never a substring/text edit on the
// original value — see normalizeComposeDockerfile's header for why that is
// unsafe.
//
// Idempotent: a fragment already shaped as `context: ${NSELF_PLUGIN_DIR}/<name>`
// + `dockerfile: Dockerfile` (or `dockerfile: Dockerfile.golang`, `dockerfile:
// Dockerfile.go` — any bare filename with no directory component) matches
// neither detection signal and is returned byte-for-byte unchanged, as is a
// plain `context: .` fragment (relied on by the 31 plugins that never had
// this bug).
func normalizeComposeBuildContext(content []byte, pluginDir, pluginName string) []byte {
	lines := strings.Split(string(content), "\n")
	changed := false

	for i := 0; i < len(lines); i++ {
		bm := composeBuildLineRE.FindStringSubmatch(lines[i])
		if bm == nil {
			continue
		}
		buildIndent := bm[1]

		// The build: sub-block runs until the next line at or above
		// buildIndent's own depth (a sibling key such as image:/ports:, or
		// the next service).
		blockEnd := len(lines)
		for j := i + 1; j < len(lines); j++ {
			trimmed := strings.TrimRight(lines[j], " \t")
			if trimmed == "" {
				continue
			}
			leading := len(lines[j]) - len(strings.TrimLeft(lines[j], " \t"))
			if leading <= len(buildIndent) {
				blockEnd = j
				break
			}
		}

		contextLine, dockerfileLine := -1, -1
		var contextIndent, contextVal, dockerfileIndent, dockerfileVal string
		for j := i + 1; j < blockEnd; j++ {
			if m := composeBuildContextRE.FindStringSubmatch(lines[j]); m != nil {
				contextLine, contextIndent, contextVal = j, m[1], m[2]
				continue
			}
			if m := composeBuildDockerfileRE.FindStringSubmatch(lines[j]); m != nil {
				dockerfileLine, dockerfileIndent, dockerfileVal = j, m[1], m[2]
			}
		}
		if contextLine == -1 || dockerfileLine == -1 {
			continue // not a simple context+dockerfile shape — nothing safe to rewrite
		}

		escapes := contextEscapesPluginDir(contextVal)
		hasDirComponent := filepath.ToSlash(filepath.Dir(dockerfileVal)) != "."
		if !escapes && !hasDirComponent {
			continue // already the installed-layout shape
		}

		resolved := resolveDockerfileName(pluginDir, pluginName, dockerfileVal)
		if resolved == "" {
			slog.Warn("plugin compose build context targets the source-repo layout and no installed Dockerfile exists to rewrite it against",
				"plugin", pluginName, "context", contextVal, "dockerfile", dockerfileVal)
			continue
		}

		lines[contextLine] = contextIndent + "context: ${NSELF_PLUGIN_DIR}/" + pluginName
		lines[dockerfileLine] = dockerfileIndent + "dockerfile: " + resolved
		changed = true
	}

	if !changed {
		return content
	}
	return []byte(strings.Join(lines, "\n"))
}

// resolveDockerfileName picks the safe replacement value for a rewritten
// build.dockerfile: the BASENAME of the originally-authored value when that
// exact file exists at the installed plugin root (e.g. a source-repo
// "paid/nself-alert-router/Dockerfile.golang" resolves to "Dockerfile.golang"
// if that file ships at the plugin root), else the canonical bare
// "Dockerfile" when that exists, else "" — signaling the caller to leave the
// block untouched and warn rather than guess. This never does a substring
// edit on the original value (see normalizeComposeDockerfile's header for
// why that is unsafe: "Dockerfile.go" is a literal prefix of
// "Dockerfile.golang").
func resolveDockerfileName(pluginDir, pluginName, original string) string {
	if base := filepath.Base(original); base != "" && base != "." && base != string(filepath.Separator) {
		if _, err := os.Stat(filepath.Join(pluginDir, pluginName, base)); err == nil {
			return base
		}
	}
	return canonicalDockerfile(pluginDir, pluginName)
}

// contextEscapesPluginDir reports whether a build.context value walks above
// its own base once ${NSELF_PLUGIN_DIR} is substituted with "." (a stand-in
// for "the directory the token expands to" — NSELF_PLUGIN_DIR is always an
// absolute, ".."-free path per ComputePluginEnvVars, so the substitution
// target's exact value never changes whether a ".." component survives
// filepath.Clean; "." is simplest). A bare relative context with no token at
// all (nself-alert-router's "../..") is checked the same way: substitution
// is a no-op when the token is absent, and the raw value is cleaned as-is.
func contextEscapesPluginDir(context string) bool {
	substituted := strings.ReplaceAll(context, "${NSELF_PLUGIN_DIR}", ".")
	cleaned := filepath.ToSlash(filepath.Clean(substituted))
	return cleaned == ".." || strings.HasPrefix(cleaned, "../")
}
