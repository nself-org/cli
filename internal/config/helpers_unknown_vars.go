package config

// Purpose: unknown-env-var detection (warnUnknownEnvVars, the allowlist parser, edit-distance suggestion) and project-root discovery, split out of helpers.go's basic env getters.
// Inputs: a map of loaded env vars, the known-var list from loader_known_vars.go, and an allowlist string (NSELF_ENV_ALLOWLIST-style).
// Outputs: stderr warnings for unrecognized env vars (with closest-match suggestions), and the resolved nSelf project root.
// Constraints: split out of helpers.go as a pure move (CLI-R12); no behavior change. Part of the CLI-R18 env-cascade surface — see cascade.go for the overall load order this feeds into.

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// userDefinedPrefixes lists env var prefixes that represent user-defined
// namespaces. Variables whose names start with one of these prefixes are
// intentionally unknown to the schema and must never trigger a warning.
var userDefinedPrefixes = []string{
	// User-defined namespaces (custom services, apps, schemas)
	"CS_",
	"FRONTEND_APP_",
	"REMOTE_SCHEMA_",
	"AUTH_", // hasura-auth accepts its full namespace through compose passthrough
	"HASURA_AUTH_",
	"INTERNAL_ROUTE_",
	"HASURA_EXTRA_",

	// Plugin-managed vars — injected directly by plugin compose templates,
	// never read by the CLI loader. Listed by plugin:
	"STRIPE_",     // stripe plugin
	"GITHUB_",     // github plugin
	"SHOPIFY_",    // shopify plugin
	"OPENSEARCH_", // search plugin (OpenSearch provider)
	"ZINC_",       // search plugin (Zinc provider)
	"SONIC_",      // search plugin (Sonic provider)

	// Legacy microservice vars (pre-CS_N system, plugin-managed)
	"NESTJS_",
	"BULLMQ_",
	"GOLANG_",
	"PYTHON_",
	"DASHBOARD_",
	"SERVICES_",
}

// WarnUnknownEnvVars emits a slog.Warn for every key in loaded that is:
//   - not present in known, AND
//   - not matched by a user-defined prefix (CS_*, FRONTEND_APP_*, etc.)
//
// It does NOT warn about known vars that are absent from loaded — doing so
// would flood output for every optional service the user hasn't enabled
// (TRAP-11).
//
// When a loaded key is within edit-distance 2 of a known var, the warning
// includes a "did_you_mean" field with the closest match.
//
// App-owned vars the schema cannot know about are declared via
// ENV_ALLOWLIST in any .env file: a comma-separated list of exact names or
// prefixes ending in "*" (e.g. ENV_ALLOWLIST=MY_APP_TOKEN,FEATURE_*).
// Allowlisted vars never warn — the documented mechanism for app-owned /
// passthrough env vars (ntask dogfood gap #19).
func warnUnknownEnvVars(loaded map[string]string, known []string) {
	// Build O(1) lookup for known vars.
	knownSet := make(map[string]bool, len(known))
	for _, k := range known {
		knownSet[k] = true
	}

	allowNames, allowPrefixes := parseEnvAllowlist(loaded["ENV_ALLOWLIST"])

	for k := range loaded {
		// Skip vars that are in the known set.
		if knownSet[k] {
			continue
		}

		// Skip vars the user allowlisted via ENV_ALLOWLIST.
		if envVarAllowlisted(k, allowNames, allowPrefixes) {
			continue
		}

		// Skip user-defined namespace prefixes.
		skip := false
		for _, pfx := range userDefinedPrefixes {
			if strings.HasPrefix(k, pfx) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		// Emit warning. Include a "did you mean" hint when a close match exists.
		suggestion := closestKnownVar(k, known)
		if suggestion != "" {
			slog.Warn("unknown env var — check spelling (ignored)",
				"var", k,
				"did_you_mean", suggestion,
			)
		} else {
			slog.Warn("unknown env var — check spelling (ignored)",
				"var", k,
			)
		}
	}
}

// parseEnvAllowlist splits an ENV_ALLOWLIST value ("A,B_*, C") into exact
// names and prefix patterns (entries ending in "*"). Empty entries are
// dropped; matching is case-sensitive like every other env var comparison.
func parseEnvAllowlist(raw string) (names map[string]bool, prefixes []string) {
	names = make(map[string]bool)
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.HasSuffix(entry, "*") {
			if pfx := strings.TrimSuffix(entry, "*"); pfx != "" {
				prefixes = append(prefixes, pfx)
			}
			continue
		}
		names[entry] = true
	}
	return names, prefixes
}

// envVarAllowlisted reports whether key matches an ENV_ALLOWLIST entry.
func envVarAllowlisted(key string, names map[string]bool, prefixes []string) bool {
	if names[key] {
		return true
	}
	for _, pfx := range prefixes {
		if strings.HasPrefix(key, pfx) {
			return true
		}
	}
	return false
}

// closestKnownVar returns the known var with the smallest Levenshtein distance
// to candidate, provided that distance is <= 2. Returns "" when no match
// is close enough.
func closestKnownVar(candidate string, known []string) string {
	bestVar := ""
	bestDist := 3 // distance > 2 means "no suggestion"

	for _, k := range known {
		d := editDistance(candidate, k)
		if d < bestDist {
			bestDist = d
			bestVar = k
		}
	}

	if bestDist <= 2 {
		return bestVar
	}
	return ""
}

// editDistance computes the Levenshtein edit distance between a and b using
// the standard two-row dynamic programming algorithm. Cost is O(len(a)*len(b)).
func editDistance(a, b string) int {
	la, lb := len(a), len(b)

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	// Base case: distance from "" to b[0..j] is j insertions.
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = minInt(del, minInt(ins, sub))
		}
		prev, curr = curr, prev
	}

	return prev[lb]
}

// minInt returns the smaller of x and y.
func minInt(x, y int) int {
	if x < y {
		return x
	}
	return y
}

// ── T38 ─────────────────────────────────────────────────────────────────────

// projectMarkerFiles are the committed cascade files whose presence marks a
// directory as an nSelf project root.
//
// Only `.env` used to count. That broke a legitimate shape the CLI-R18 cascade
// made first-class: a repository that commits `.env.dev` (or a per-environment
// file) and no bare `.env` is a complete project, but the CLI reported "no
// nself project found. Run 'nself init'" — advice that would have overwritten
// a working configuration. Reported by the ntask clean-fork self-host drill,
// 2026-08-24.
//
// Deliberately excludes the never-committed layers (.env.secrets, .env.local):
// those are local overlays, and a directory containing only an overlay is not
// a project anyone checked out.
var projectMarkerFiles = []string{".env", ".env.dev", ".env.staging", ".env.prod"}

// hasProjectMarker reports whether dir contains any committed cascade file.
func hasProjectMarker(dir string) bool {
	for _, name := range projectMarkerFiles {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

// FindNSelfRoot walks up from startDir looking for a nself project root.
// It checks, at each directory level:
//  1. startDir/.backend/.env  → returns startDir/.backend (monorepo case)
//  2. startDir/.env           → returns startDir (already in backend dir)
//
// Walking stops at $HOME, at /, or after 10 levels — whichever comes first.
// Returns an error if no project root is found.
func FindNSelfRoot(startDir string) (string, error) {
	home, _ := os.UserHomeDir()

	dir := startDir
	for i := 0; i < 10; i++ {
		// Stop at home or filesystem root.
		// filepath.Dir(dir) == dir covers both Unix "/" and Windows drive roots
		// like "C:\" where filepath.Dir("C:\\") == "C:\\" (unlike "/" which
		// does not match Windows drive roots).
		if dir == home || filepath.Dir(dir) == dir {
			return "", fmt.Errorf("no nself project found: looked for %s in %s and each parent directory",
				strings.Join(projectMarkerFiles, ", "), startDir)
		}

		// Monorepo case: a committed cascade file exists one level down.
		if hasProjectMarker(filepath.Join(dir, ".backend")) {
			return filepath.Join(dir, ".backend"), nil
		}

		// Already-in-backend case: a committed cascade file is right here.
		if hasProjectMarker(dir) {
			return dir, nil
		}

		// Walk up one level.
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root without hitting home guard above.
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("no nself project found")
}
