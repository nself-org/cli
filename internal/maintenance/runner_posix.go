//go:build darwin || linux

package maintenance

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RunnerRoot describes one discovered GitHub Actions self-hosted runner install.
type RunnerRoot struct {
	// Name is the systemd unit name when discovered via systemd, or the directory
	// basename when discovered via the fallback glob list.
	Name string
	// Path is the runner's install directory (what the runner config calls "work
	// folder parent" — it contains _work, _diag, bin, etc.).
	Path string
}

// protectedRunnerSubdirs are runner-internal directories under "<root>/_work" that
// must NEVER be removed by disk-cleanup, at any tier, busy or idle, under pressure or
// not. actions/runner re-downloads _actions and _tool lazily only when the directory
// is entirely missing at job start — deleting _actions out from under a *live* job
// (2026-09-11 incident: four runners on nSelf staging hit 100% disk; a naive cleanup
// wrapper race-deleted runner-3's _actions mid-job and it died mid-self-update,
// stalling every queued CI check org-wide) breaks that job hard, mid-run, with no
// chance to recover. _temp and _PipelineMapping are the same category of runner state.
var protectedRunnerSubdirs = map[string]bool{
	"_actions":         true,
	"_tool":            true,
	"_temp":            true,
	"_PipelineMapping": true,
}

// protectedCacheSubpaths (relative to a runner user's $HOME/.cache) must never be
// removed while ANY runner on the box is busy, at any tier including the disk-pressure
// escalation tier — because these are read live during an in-flight command, not just
// at job start:
//   - go-build: removing it mid-compile produced
//     "could not import fmt (open .../go-build/...)" and broke plugins-pro#114.
//   - grype, trivy: removing their DB mid-scan produced "database does not exist" and
//     broke plugins-pro#113's SBOM step.
var protectedCacheSubpaths = []string{
	filepath.Join(".cache", "go-build"),
	filepath.Join(".cache", "grype"),
	filepath.Join(".cache", "trivy"),
}

func isProtectedCachePath(relPath string) bool {
	for _, p := range protectedCacheSubpaths {
		if relPath == p {
			return true
		}
	}
	return false
}

// dirSize returns the total size in bytes of all regular files under root. It is
// best-effort: unreadable entries are skipped rather than aborting the walk, since a
// cleanup pass should never fail just because it couldn't stat one stale file.
func dirSize(root string) int64 {
	var total int64
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil //nolint:nilerr
		}
		total += info.Size()
		return nil
	})
	return total
}

// reclaimRunnerWork removes job checkout directories under "<root>/_work" for one
// runner — the caller (DiskCleanupWithOptions) has already established that this
// specific runner root is idle before calling this. It walks only direct children of
// _work and skips protectedRunnerSubdirs individually, so a mis-named or unexpected
// entry never causes the whole _work tree to be skipped or removed wholesale.
func reclaimRunnerWork(root string, dryRun bool) (bytes int64, reclaimed []ReclaimEntry, skipped []SkipEntry) {
	workDir := filepath.Join(root, "_work")
	entries, err := os.ReadDir(workDir)
	if err != nil {
		if os.IsNotExist(err) {
			// No _work dir yet (fresh install). "Absent" and "already clean"
			// are the same answer to every caller: there is nothing to reclaim
			// and nothing was deliberately left behind.
			return 0, nil, nil
		}
		// Any other read failure (ENOTDIR, EACCES, EIO) is NOT "nothing to do".
		// Returning a bare zero made a runner root we could not even open look
		// identical to one that was already clean, so a cleanup pass that
		// reclaimed nothing because it was locked out logged the same
		// "0 bytes, nothing skipped" as a healthy no-op. Recording it as a skip
		// is what SkipEntry exists for — see its doc comment in disk_shared.go.
		return 0, nil, []SkipEntry{{
			Path:   workDir,
			Reason: fmt.Sprintf("could not read runner work directory: %v", err),
		}}
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		full := filepath.Join(workDir, name)

		if protectedRunnerSubdirs[name] {
			skipped = append(skipped, SkipEntry{
				Path:   full,
				Reason: "runner-internal directory, never removed (_actions/_tool/_temp/_PipelineMapping)",
			})
			continue
		}

		// A per-runner busy check is necessary but not sufficient: the check
		// and the removal are not atomic. On 2026-09-11, cleaning workspaces
		// on runners that reported idle destroyed three live jobs, which
		// failed with "Directory .../_work/web/web does not exist" — a job had
		// started in the window between the check and the RemoveAll.
		//
		// A running job writes into its workspace constantly, so recent
		// modification is the signal a process scan cannot give us. Require
		// the tree to have been untouched for workspaceStaleAfter before
		// removing it. A merely slow job still touches its workspace; a
		// finished one cannot.
		if recentlyModified(full, workspaceStaleAfter) {
			skipped = append(skipped, SkipEntry{
				Path:   full,
				Reason: fmt.Sprintf("modified within %s — treated as an active job workspace", workspaceStaleAfter),
			})
			continue
		}

		size := dirSize(full)
		if !dryRun {
			if err := os.RemoveAll(full); err != nil {
				skipped = append(skipped, SkipEntry{Path: full, Reason: fmt.Sprintf("remove failed: %v", err)})
				continue
			}
		}
		bytes += size
		reclaimed = append(reclaimed, ReclaimEntry{Path: full, Bytes: size, Tier: tierIdlePerRunner})
	}
	return bytes, reclaimed, skipped
}

// reclaimCaches removes regenerable build/package caches under home, plus any
// sharedRoots that exist on this box. It never descends into protectedCacheSubpaths —
// see that var's doc comment. sharedRoots is a parameter (rather than always reading
// the package-level sharedCacheRoots) so tests can pass an empty list and guarantee
// nothing outside the test's own fixture tree is ever touched.
func reclaimCaches(home string, sharedRoots []string, dryRun bool) (bytes int64, reclaimed []ReclaimEntry, skipped []SkipEntry) {
	// .cache is walked one level at a time so protected subdirectories can be
	// skipped individually instead of skipping the whole tree.
	cacheDir := filepath.Join(home, ".cache")
	if entries, err := os.ReadDir(cacheDir); err == nil {
		for _, e := range entries {
			full := filepath.Join(cacheDir, e.Name())
			rel := filepath.Join(".cache", e.Name())
			if isProtectedCachePath(rel) {
				skipped = append(skipped, SkipEntry{
					Path:   full,
					Reason: "compiler/scanner input, never removed (go-build/grype/trivy)",
				})
				continue
			}
			b, r, s := reclaimPath(full, tierIdleShared, dryRun)
			bytes += b
			reclaimed = append(reclaimed, r...)
			skipped = append(skipped, s...)
		}
	}

	// Module/package caches: regenerate automatically on next fetch, so they are
	// safe to remove wholesale (unlike the compiler/scanner caches above, which are
	// read live mid-command).
	for _, rel := range []string{filepath.Join("go", "pkg", "mod"), "pnpm-store"} {
		full := filepath.Join(home, rel)
		b, r, s := reclaimPath(full, tierIdleShared, dryRun)
		bytes += b
		reclaimed = append(reclaimed, r...)
		skipped = append(skipped, s...)
	}

	for _, full := range sharedRoots {
		b, r, s := reclaimPath(full, tierIdleShared, dryRun)
		bytes += b
		reclaimed = append(reclaimed, r...)
		skipped = append(skipped, s...)
	}

	return bytes, reclaimed, skipped
}

// sharedCacheRoots are absolute (non-home-relative) cache locations also safe to
// reclaim under the same rules as reclaimCaches' home-relative targets.
var sharedCacheRoots = []string{
	"/opt/pnpm-store",
}

// reclaimPath removes one path in full if it exists and is a directory, or reports it
// as skipped on stat/remove failure. Missing paths are simply omitted (not an error —
// not every box has every cache).
func reclaimPath(full string, tier reclaimTier, dryRun bool) (int64, []ReclaimEntry, []SkipEntry) {
	info, err := os.Stat(full)
	if err != nil || !info.IsDir() {
		return 0, nil, nil
	}
	size := dirSize(full)
	if !dryRun {
		if err := os.RemoveAll(full); err != nil {
			return 0, nil, []SkipEntry{{Path: full, Reason: fmt.Sprintf("remove failed: %v", err)}}
		}
	}
	return size, []ReclaimEntry{{Path: full, Bytes: size, Tier: tier}}, nil
}

// listRunnerWorkerProcesses returns the full command-line of every currently running
// process on the box, one process per element. It is a var so tests can replace it
// with a fixture instead of shelling out to `ps` — busy detection must be testable
// without a real runner process anywhere near the test box.
var listRunnerWorkerProcesses = func() ([]string, error) {
	out, err := runCommand("ps", "-eo", "args=")
	if err != nil {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

// isRunnerBusy reports whether the given runner root currently has a live
// Runner.Worker process. It matches on the worker process's own executable path
// containing the runner root — each self-hosted runner's worker binary lives at
// "<root>/bin/Runner.Worker", so a busy runner's process line always contains its own
// root path even when several runners' workers are running side by side on the same
// box.
//
// This fixes the wrapper that shipped the original guard: it checked
// `pgrep -f "Runner.Worker"` globally, so ONE busy runner out of four skipped cleanup
// for ALL four runners — on a farm that is busy around the clock, the cleanup path
// never ran at all.
func isRunnerBusy(root string) bool {
	lines, err := listRunnerWorkerProcesses()
	if err != nil {
		// Can't tell — assume busy. Skipping a cleanup pass is always recoverable;
		// deleting a live job's workspace is not.
		return true
	}
	for _, line := range lines {
		if strings.Contains(line, "Runner.Worker") && strings.Contains(line, root) {
			return true
		}
	}
	return false
}

// workspaceStaleAfter is how long a runner job workspace must go untouched
// before disk-cleanup will treat it as abandoned and remove it. This exists
// because isRunnerBusy alone races: a runner can report idle and begin a job
// microseconds later. 30 minutes is comfortably longer than the gap between a
// job's filesystem writes while it runs, and far shorter than the lifetime of
// a genuinely abandoned checkout.
// It is a var, not a const, so tests can exercise both sides of the guard
// without having to backdate every fixture they build.
var workspaceStaleAfter = 30 * time.Minute

// recentlyModified reports whether any file under root was modified within the
// last d. It stops at the first hit rather than walking the whole tree, since
// one recent file is enough to prove the workspace is in use.
func recentlyModified(root string, d time.Duration) bool {
	cutoff := time.Now().Add(-d)
	found := false
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entry proves nothing; keep looking
		}
		info, statErr := entry.Info()
		if statErr != nil {
			return nil
		}
		if info.ModTime().After(cutoff) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}
