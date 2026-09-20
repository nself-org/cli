package build

// Purpose: rewrites a plugin compose fragment's build.context/build.dockerfile
// pair when the context was authored for the SOURCE REPO layout instead of
// the INSTALLED layout.
// Inputs: a compose fragment's bytes plus the plugin directory/name pair
// already threaded through DiscoverPluginComposeFiles (plugins.go).
// Outputs: the (possibly rewritten) compose fragment bytes.
// Constraints:
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

import (
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

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

// normalizeComposeBuildContext rewrites a plugin compose fragment's
// build.context/build.dockerfile pair when the context was authored for the
// source-repo layout instead of the installed layout. See the file header
// for the full detection/rewrite rationale.
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
