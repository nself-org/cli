//go:build darwin || linux

package maintenance

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// DefaultPressureThreshold is the disk-used percentage at/above which idle-shared
// cache reclaims run regardless of runner busy state. A full disk fails every job on
// the box anyway, so waiting for idle past this point is strictly worse than
// reclaiming now.
//
// 75, not 85, because this number has to keep a DIFFERENT check satisfied:
// `nself doctor --deep` fails the host with "Disk free: /: N% free (<20%)" at
// 80% used. An escalation threshold above that lets the box settle in a band
// where cleanup is content but doctor is red — which is exactly what happened
// on nSelf staging on 2026-09-11, where the dogfood gate failed on disk while
// the daily cleanup timer reported nothing to do. Escalating at 75 keeps the
// box under doctor's limit with headroom for one large job's working set.
//
// Keep this BELOW the doctor host-disk threshold. If that check's limit moves,
// move this with it.
const DefaultPressureThreshold = 75

// GetDiskUsage returns current disk utilisation for the root filesystem ("/").
func GetDiskUsage() (DiskUsage, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return DiskUsage{}, fmt.Errorf("statfs /: %w", err)
	}

	blockSize := uint64(stat.Bsize)
	totalBytes := stat.Blocks * blockSize
	freeBytes := uint64(stat.Bavail) * blockSize
	usedBytes := totalBytes - freeBytes

	const gb = 1024 * 1024 * 1024
	totalGB := float64(totalBytes) / gb
	freeGB := float64(freeBytes) / gb
	usedGB := float64(usedBytes) / gb

	var usedPct int
	if totalBytes > 0 {
		usedPct = int((usedBytes * 100) / totalBytes)
	}

	return DiskUsage{
		UsedPercent: usedPct,
		TotalGB:     totalGB,
		UsedGB:      usedGB,
		FreeGB:      freeGB,
	}, nil
}

// DiskCleanupOptions configures a DiskCleanup run.
type DiskCleanupOptions struct {
	// DryRun reports what would be removed and the space it would free, without
	// removing anything.
	DryRun bool
	// PressureThreshold is the disk-used percentage at/above which idle-shared
	// reclaims run even while a runner is busy. Zero means DefaultPressureThreshold.
	PressureThreshold int
	// Home overrides the home directory used to locate caches. Defaults to $HOME
	// (falling back to os.UserHomeDir()). Tests inject this to point at a fixture
	// tree instead of the real user's home.
	Home string
	// RunnerRoots overrides runner discovery. Tests inject this to point at fixture
	// runner trees instead of discovering real installs via systemd/globs.
	RunnerRoots []RunnerRoot
	// SharedCacheRoots overrides the absolute (non-home-relative) cache locations
	// considered for reclaim (default: sharedCacheRoots, e.g. "/opt/pnpm-store").
	// Tests set this to an empty (non-nil) slice to guarantee nothing outside a
	// fixture tree is ever touched; nil means "use the default list".
	SharedCacheRoots []string
	// UsageOverride, when non-nil, is used instead of calling GetDiskUsage() for the
	// "before" reading that pressure-escalation compares against threshold. Tests use
	// this for deterministic threshold behavior instead of depending on the test
	// machine's real, unpredictable disk usage.
	UsageOverride *DiskUsage
}

// DiskCleanup runs the full cleanup with default options: not a dry run, the default
// pressure threshold, real runner discovery, and $HOME for caches.
func DiskCleanup() CleanupResult {
	return DiskCleanupWithOptions(DiskCleanupOptions{})
}

// DiskCleanupDryRun runs the full cleanup in dry-run mode: nothing is removed, but the
// returned CleanupResult's Reclaimed/Skipped/BytesReclaimed report exactly what a real
// run would have done and why anything was left alone.
func DiskCleanupDryRun() CleanupResult {
	return DiskCleanupWithOptions(DiskCleanupOptions{DryRun: true})
}

// DiskCleanupWithOptions runs disk-cleanup with explicit options. It never aborts
// early — it collects all errors/skips and reports at the end, tier by tier:
//
//  1. tierAlways — docker dangling image/build-cache/anonymous-volume prune, old
//     compressed log rotation, journald vacuum. Runs unconditionally.
//  2. tierIdlePerRunner — GitHub Actions runner job workspace directories under
//     "<root>/_work", one runner root at a time. Only runs for a runner root that
//     isRunnerBusy reports idle; a busy runner's workspace is always left alone,
//     pressure or not, because deleting an in-progress job's own checkout breaks
//     that job outright.
//  3. tierIdleShared — regenerable package/module caches (go build cache excluded —
//     see protectedCacheSubpaths). Prefers every runner being idle, but runs anyway
//     once disk usage is at/above PressureThreshold.
//
// protectedRunnerSubdirs and protectedCacheSubpaths are excluded at every tier,
// unconditionally — see their doc comments in runner_posix.go for the incidents that
// made them hard exclusions rather than a "prefer not to" default.
func DiskCleanupWithOptions(opts DiskCleanupOptions) CleanupResult {
	result := CleanupResult{DryRun: opts.DryRun}

	threshold := opts.PressureThreshold
	if threshold <= 0 {
		threshold = DefaultPressureThreshold
	}

	home := opts.Home
	if home == "" {
		home = os.Getenv("HOME")
	}
	if home == "" {
		if h, err := os.UserHomeDir(); err == nil {
			home = h
		}
	}

	var before DiskUsage
	if opts.UsageOverride != nil {
		before = *opts.UsageOverride
	} else {
		b, err := GetDiskUsage()
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("read disk usage (before): %w", err))
		}
		before = b
	}
	result.Before = before

	underPressure := before.UsedPercent >= threshold

	// Tier: always safe.
	dockerOut, dockerErrs := dockerReclaimFunc(opts.DryRun)
	result.DockerPruneOut = dockerOut
	result.Errors = append(result.Errors, dockerErrs...)

	logOut, logErr := logRotationFunc(opts.DryRun)
	result.LogRotationOut = logOut
	if logErr != nil {
		// non-fatal — /var/log may not exist on all platforms
		result.Errors = append(result.Errors, fmt.Errorf("log rotation: %w", logErr))
	}

	journalOut, journalErr := journalVacuumFunc(opts.DryRun)
	result.JournalVacuumOut = journalOut
	if journalErr != nil {
		// non-fatal on macOS (no journald)
		result.Errors = append(result.Errors, fmt.Errorf("journalctl vacuum: %w", journalErr))
	}

	// Tier: per-runner idle-gated job workspaces.
	roots := opts.RunnerRoots
	if roots == nil {
		roots = discoverRunnerRoots()
	}
	anyBusy := false
	for _, root := range roots {
		if isRunnerBusy(root.Path) {
			anyBusy = true
			result.Skipped = append(result.Skipped, SkipEntry{
				Path:   root.Path,
				Reason: "runner busy (Runner.Worker running) — job workspace left alone",
			})
			continue
		}
		b, reclaimed, skipped := reclaimRunnerWork(root.Path, opts.DryRun)
		result.BytesReclaimed += b
		result.Reclaimed = append(result.Reclaimed, reclaimed...)
		result.Skipped = append(result.Skipped, skipped...)
	}

	// Tier: shared idle-preferred caches, escalated by disk pressure.
	if home != "" {
		if !anyBusy || underPressure {
			sharedRoots := opts.SharedCacheRoots
			if sharedRoots == nil {
				sharedRoots = sharedCacheRoots
			}
			b, reclaimed, skipped := reclaimCaches(home, sharedRoots, opts.DryRun)
			result.BytesReclaimed += b
			result.Reclaimed = append(result.Reclaimed, reclaimed...)
			result.Skipped = append(result.Skipped, skipped...)
		} else {
			result.Skipped = append(result.Skipped, SkipEntry{
				Path:   filepath.Join(home, ".cache"),
				Reason: "runner(s) busy and disk usage below pressure threshold — shared caches left alone",
			})
		}
	}

	after, err := GetDiskUsage()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("read disk usage (after): %w", err))
	}
	result.After = after

	return result
}

// logRotationFunc and journalVacuumFunc are vars (not plain function calls) so tests
// can stub them out — disk-cleanup tests must never shell out to real `find`/
// `journalctl` against the test box's actual /var/log or journald state.
var (
	logRotationFunc   = logRotation
	journalVacuumFunc = journalVacuum
)

// logRotation deletes compressed logs older than 14 days under /var/log. In dry-run
// mode it lists what would be deleted (via `find` without `-delete`) instead.
func logRotation(dryRun bool) (string, error) {
	if dryRun {
		out, err := runCommand("find", "/var/log", "-name", "*.gz", "-mtime", "+14")
		return "would delete:\n" + out, err
	}
	return runCommand("find", "/var/log", "-name", "*.gz", "-mtime", "+14", "-delete")
}

// journalVacuum runs `journalctl --vacuum-time=7d` (Linux only; harmless no-op on
// macOS, where the command doesn't exist and the resulting error is treated as
// non-fatal by the caller). In dry-run mode it does nothing — journalctl has no
// built-in dry-run, and vacuuming is already always-safe, so there's nothing
// meaningful to preview.
func journalVacuum(dryRun bool) (string, error) {
	if dryRun {
		return "dry-run: journalctl --vacuum-time=7d (skipped, always-safe tier)", nil
	}
	return runCommand("journalctl", "--vacuum-time=7d")
}
