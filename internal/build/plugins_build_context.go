package build

// Purpose: rewrites a plugin compose fragment's build.context (mapping form
// or single-line string form) plus its build.dockerfile when the context is
// anything other than the canonical installed-plugin-dir form.
// Inputs: a compose fragment's bytes plus the plugin directory/name pair
// already threaded through DiscoverPluginComposeFiles (plugins.go).
// Outputs: the (possibly rewritten) compose fragment bytes.
// Constraints:
//
// WHY: `nself start`/`nself build` invoke `docker compose -f
// <project>/docker-compose.yml -f ~/.nself/plugins/<name>/docker-compose.
// plugin.yml ... up -d` with no `--project-directory` flag (see
// internal/docker/compose.go buildBaseArgs). Docker Compose resolves EVERY
// relative build.context in EVERY merged -f file against the directory of
// the FIRST -f file — the project's docker-compose.yml — never against the
// file that actually declared the context. A plugin fragment's own
// directory is therefore never the resolution base for anything relative it
// writes, no matter how "correct" that value looks in isolation: a bare
// `context: .` resolves to the PROJECT directory, not the plugin's; a
// `context: ./web` or `context: web` the same, one level down from the
// project dir instead of the plugin's. This is true of every relative
// value — there is no shape of relative context that survives being merged
// as a non-first -f file. The only value that resolves correctly regardless
// of -f ordering is one that names the plugin's installed directory
// explicitly: `context: ${NSELF_PLUGIN_DIR}/<name>[/<subpath>]` (ai/claw/mux
// /voice already ship this shape). This rewrite converges every plugin onto
// that shape.
//
// (Live repro, 2026-09-20: `docker compose -f project/docker-compose.yml -f
// ~/.nself/plugins/browser/docker-compose.plugin.yml config` with browser's
// `build.context: .` echoes the project directory as the resolved context,
// not ~/.nself/plugins/browser — confirmed against browser, google, and ~29
// other licensed plugins that ship the identical bare `.` shape. `nself
// start` then fails opaquely: "failed to read dockerfile: open Dockerfile:
// no such file or directory" — E2E golden path step 13, released v1.4.2.)
//
// A prior version of this rewrite treated `context: .` (and any other
// relative value that stayed inside the plugin's own directory once
// resolved against it) as "already correct" and left it untouched — that
// was the defect: it assumed the plugin fragment's own directory is a valid
// resolution base, which Compose's multi -f-file merge semantics never
// honor. There is no such thing as a relative context that "never had this
// bug" — 31 plugins shipping a bare `.` were simply never exercised by the
// E2E golden path's plugin-build step until browser/google were.
//
// Detection: any build.context value that is not already the canonical
// `${NSELF_PLUGIN_DIR}/<pluginName>[/<subpath>]` shape and is not an
// absolute filesystem path is rewritten — this covers `.`, `./sub`, `sub`,
// a `${NSELF_PLUGIN_DIR}/../..`-style escape, and a bare `../..` escape
// alike. A build.dockerfile that names a directory (anything but a bare
// filename) is also rewritten — a correctly-scoped context never needs one,
// since Docker resolves dockerfile relative to context.
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
// (optionally + `/<subpath>`) + a bare-filename `dockerfile:` matches
// neither rewrite signal and is returned byte-for-byte unchanged.

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

// composeBuildStringRE matches a service-level `build:` declared as a
// single-line string (the compose shorthand for "context only, dockerfile
// defaults to Dockerfile at that context's root") rather than a mapping,
// e.g. `  widget:\n    build: .` or `    build: ./web`. Captures indentation
// (group 1) and the raw context value (group 2). Deliberately disjoint from
// composeBuildLineRE (which requires nothing after the colon).
var composeBuildStringRE = regexp.MustCompile(`^(\s*)build:\s+(\S+)\s*$`)

// composeBuildContextRE and composeBuildDockerfileRE match a `context:` /
// `dockerfile:` scalar line inside a build: block, capturing indentation
// (group 1, reused verbatim on rewrite so we never guess at the author's
// spacing) and the raw value (group 2).
var composeBuildContextRE = regexp.MustCompile(`^(\s*)context:\s*(\S+)\s*$`)
var composeBuildDockerfileRE = regexp.MustCompile(`^(\s*)dockerfile:\s*(\S+)\s*$`)

// normalizeComposeBuildContext rewrites a plugin compose fragment's
// build.context/build.dockerfile pair — mapping form or single-line string
// form — to the canonical installed-plugin-dir shape. See the file header
// for the full detection/rewrite rationale.
func normalizeComposeBuildContext(content []byte, pluginDir, pluginName string) []byte {
	lines := strings.Split(string(content), "\n")
	changed := false

	for i := 0; i < len(lines); i++ {
		if bm := composeBuildLineRE.FindStringSubmatch(lines[i]); bm != nil {
			if rewriteBuildMappingBlock(lines, i, bm[1], pluginDir, pluginName) {
				changed = true
			}
			continue
		}
		if sm := composeBuildStringRE.FindStringSubmatch(lines[i]); sm != nil {
			indent, contextVal := sm[1], sm[2]
			newVal, ctxChanged := normalizeBuildContextValue(contextVal, pluginName)
			if !ctxChanged {
				continue
			}
			lines[i] = indent + "build: " + newVal
			changed = true
		}
	}

	if !changed {
		return content
	}
	return []byte(strings.Join(lines, "\n"))
}

// rewriteBuildMappingBlock handles one `build:` mapping block starting at
// lines[buildLineIdx]. It finds the block's context:/dockerfile: pair,
// decides whether either needs rewriting, and mutates lines in place.
// Returns whether it changed anything.
func rewriteBuildMappingBlock(lines []string, buildLineIdx int, buildIndent, pluginDir, pluginName string) bool {
	// The build: sub-block runs until the next line at or above
	// buildIndent's own depth (a sibling key such as image:/ports:, or the
	// next service).
	blockEnd := len(lines)
	for j := buildLineIdx + 1; j < len(lines); j++ {
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
	for j := buildLineIdx + 1; j < blockEnd; j++ {
		if m := composeBuildContextRE.FindStringSubmatch(lines[j]); m != nil {
			contextLine, contextIndent, contextVal = j, m[1], m[2]
			continue
		}
		if m := composeBuildDockerfileRE.FindStringSubmatch(lines[j]); m != nil {
			dockerfileLine, dockerfileIndent, dockerfileVal = j, m[1], m[2]
		}
	}
	if contextLine == -1 || dockerfileLine == -1 {
		return false // not a simple context+dockerfile shape — nothing safe to rewrite
	}

	newContextVal, ctxChanged := normalizeBuildContextValue(contextVal, pluginName)
	hasDirComponent := filepath.ToSlash(filepath.Dir(dockerfileVal)) != "."
	if !ctxChanged && !hasDirComponent {
		return false // already the canonical installed-layout shape
	}

	resolved := resolveDockerfileName(pluginDir, pluginName, dockerfileVal)
	if resolved == "" {
		slog.Warn("plugin compose build context is not the canonical installed-plugin-dir shape and no installed Dockerfile exists to rewrite it against",
			"plugin", pluginName, "context", contextVal, "dockerfile", dockerfileVal)
		return false
	}

	lines[contextLine] = contextIndent + "context: " + newContextVal
	lines[dockerfileLine] = dockerfileIndent + "dockerfile: " + resolved
	return true
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

// normalizeBuildContextValue computes the canonical replacement for a
// build.context value and reports whether it differs from the original.
// The canonical shape is `${NSELF_PLUGIN_DIR}/<pluginName>[/<subpath>]` —
// the only context form that resolves correctly regardless of which -f
// file Compose treats as "first" (see file header). Two shapes are left
// untouched:
//   - an absolute filesystem path (starts with "/") — not project-relative
//     at all, so the multi -f-file merge issue this rewrite targets does
//     not apply to it, and rewriting it would silently redirect a
//     deliberately external build context.
//   - the canonical shape itself, `${NSELF_PLUGIN_DIR}/<pluginName>`
//     optionally followed by a subpath — already correct and idempotent.
//
// Every other relative shape is rewritten to the canonical form: `.`
// collapses to no subpath, `./sub` and `sub` keep "sub" as the preserved
// subpath, and any value that (once ${NSELF_PLUGIN_DIR} is accounted for)
// cleans to a path walking above the plugin's own directory — the
// `${NSELF_PLUGIN_DIR}/../..` and bare `../..` shapes authored against the
// SOURCE REPO layout — collapses to no subpath, since the escape means the
// original value cannot be trusted to describe a real subpath; the
// dockerfile's own basename (resolved separately by resolveDockerfileName)
// carries the actual file to build from.
func normalizeBuildContextValue(contextVal, pluginName string) (string, bool) {
	if strings.HasPrefix(contextVal, "/") {
		return contextVal, false // absolute filesystem path — leave alone
	}

	var rel string
	escaped := false

	if strings.HasPrefix(contextVal, "${NSELF_PLUGIN_DIR}") {
		remainder := strings.TrimPrefix(strings.TrimPrefix(contextVal, "${NSELF_PLUGIN_DIR}"), "/")
		switch {
		case remainder == pluginName:
			rel = "."
		case strings.HasPrefix(remainder, pluginName+"/"):
			rel = strings.TrimPrefix(remainder, pluginName+"/")
		default:
			// ${NSELF_PLUGIN_DIR} expands to the GLOBAL plugin dir, not
			// this plugin's own directory or the source-repo tree — a
			// remainder that isn't "<pluginName>" or "<pluginName>/..."
			// (e.g. "../..") always escapes at runtime.
			escaped = true
		}
	} else {
		rel = contextVal
	}

	if !escaped {
		cleaned := filepath.ToSlash(filepath.Clean(rel))
		switch {
		case cleaned == ".." || strings.HasPrefix(cleaned, "../"):
			escaped = true
		case cleaned == ".":
			rel = ""
		default:
			rel = cleaned
		}
	}
	if escaped {
		rel = ""
	}

	newContext := "${NSELF_PLUGIN_DIR}/" + pluginName
	if rel != "" {
		newContext += "/" + rel
	}
	return newContext, newContext != contextVal
}
