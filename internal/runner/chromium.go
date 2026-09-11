package runner

// Purpose: the check that catches the specific failure mode that hid for
//   hours on 2026-09-11 — `npx playwright install chromium` (the fallback
//   path when `--with-deps` fails because sudo needs a terminal) downloads
//   a Chromium binary with none of its shared libraries, so the browser
//   crashes with `chrome: error while loading shared libraries:
//   libnspr4.so`, which Playwright then reports as the generic, misleading
//   "Target page, context or browser has been closed". Running `ldd` on
//   any cached Chromium and grepping for "not found" catches this directly
//   instead of waiting for a job to hit it.
// Inputs:  Manifest.ChromiumCacheGlobs (home-relative glob patterns) and an
//   Executor.
// Outputs: one CheckResult per cached Chromium/headless-shell binary found
//   across every home directory on the host, plus a single StatusWarn
//   result when none is cached yet (not itself a failure — Playwright may
//   simply never have run there).
// Constraints: expansion happens in the remote/local shell (bash globbing
//   with nullglob), not in Go, because Executor has no directory-listing
//   primitive of its own and adding one would duplicate what the shell
//   already does correctly.

import (
	"context"
	"strings"
)

const chromiumWarnName = "chromium:cache"

// checkChromiumLdd finds every cached Chromium/headless-shell binary under
// each home directory and runs `ldd` against it, reporting any shared
// library `ldd` could not resolve.
func checkChromiumLdd(ctx context.Context, ex Executor, m *Manifest) []CheckResult {
	if len(m.ChromiumCacheGlobs) == 0 {
		return nil
	}
	out, err := ex.Run(ctx, chromiumLddScript(m.ChromiumCacheGlobs))
	if err != nil && out == "" {
		return []CheckResult{{Name: chromiumWarnName, Status: StatusWarn,
			Detail: "could not scan for cached Chromium: " + err.Error()}}
	}
	return parseChromiumLddOutput(out)
}

// chromiumLddScript builds the shell script that walks every home
// directory, expands each glob (with %h substituted for the loop
// variable), and ldd's anything executable it finds.
func chromiumLddScript(globs []string) string {
	var patterns []string
	for _, g := range globs {
		patterns = append(patterns, strings.ReplaceAll(g, "%h", "$home"))
	}
	return "shopt -s nullglob; for home in /root /home/*; do for f in " +
		strings.Join(patterns, " ") +
		`; do [ -x "$f" ] || continue; echo "CHROME $f"; ldd "$f" 2>&1 | grep "not found" | sed "s/^/MISSING /"; done; done`
}

// parseChromiumLddOutput turns chromiumLddScript's "CHROME <path>" /
// "MISSING <ldd line>" output into one CheckResult per binary found, or a
// single StatusWarn result when nothing was cached.
func parseChromiumLddOutput(out string) []CheckResult {
	var results []CheckResult
	var current *CheckResult
	var missingLibs []string
	flush := func() {
		if current == nil {
			return
		}
		if len(missingLibs) > 0 {
			current.Status = StatusFail
			current.Detail = "missing: " + strings.Join(missingLibs, "; ")
		}
		results = append(results, *current)
	}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "CHROME "):
			flush()
			path := strings.TrimPrefix(line, "CHROME ")
			missingLibs = nil
			current = &CheckResult{Name: "chromium:ldd:" + path, Status: StatusPass,
				Detail: path + " resolves all shared libraries"}
		case strings.HasPrefix(line, "MISSING ") && current != nil:
			missingLibs = append(missingLibs, strings.TrimPrefix(line, "MISSING "))
		}
	}
	flush()
	if len(results) == 0 {
		return []CheckResult{{Name: chromiumWarnName, Status: StatusWarn,
			Detail: "no cached Chromium found under any home directory (Playwright has not run here yet)"}}
	}
	return results
}
