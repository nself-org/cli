//go:build darwin || linux

package maintenance

import (
	"os"
	"path/filepath"
	"strings"
)

// runnerRootsEnvVar overrides discovery entirely with an explicit, colon-separated
// list of runner install directories. Useful when systemd unit naming doesn't match
// the default convention, or when systemctl isn't usable (containers, restricted
// environments).
const runnerRootsEnvVar = "NSELF_MAINTENANCE_RUNNER_ROOTS"

// defaultRunnerRootGlobs are the conventional self-hosted runner install locations
// used across the nself fleet. Documented here rather than hardcoded to one path so a
// box with runners under /opt, or under a non-"runner" username, is still discovered
// even without systemd unit introspection.
var defaultRunnerRootGlobs = []string{
	"/home/*/actions-runner*",
	"/home/*/*/actions-runner*",
	"/opt/actions-runner*",
}

// discoverRunnerRoots finds installed GitHub Actions self-hosted runners. It prefers
// systemd unit discovery (unit names of the form "actions.runner.*"), which is
// accurate even for non-default install paths, and falls back to
// defaultRunnerRootGlobs when systemctl is unavailable or returns nothing (macOS dev
// boxes, containers). NSELF_MAINTENANCE_RUNNER_ROOTS, when set, short-circuits both.
func discoverRunnerRoots() []RunnerRoot {
	if override := os.Getenv(runnerRootsEnvVar); override != "" {
		var roots []RunnerRoot
		for _, p := range strings.Split(override, ":") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			roots = append(roots, RunnerRoot{Name: filepath.Base(p), Path: p})
		}
		return roots
	}

	if roots := discoverRunnerRootsFromSystemd(); len(roots) > 0 {
		return roots
	}
	return discoverRunnerRootsFromGlobs()
}

func discoverRunnerRootsFromSystemd() []RunnerRoot {
	out, err := runCommand("systemctl", "list-units", "--type=service", "--all", "--no-legend", "--plain")
	if err != nil {
		return nil
	}
	var roots []RunnerRoot
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		unit := fields[0]
		if !strings.HasPrefix(unit, "actions.runner.") {
			continue
		}
		wd, err := runCommand("systemctl", "show", unit, "-p", "WorkingDirectory", "--value")
		if err != nil {
			continue
		}
		wd = strings.TrimSpace(wd)
		if wd == "" {
			continue
		}
		roots = append(roots, RunnerRoot{Name: unit, Path: wd})
	}
	return roots
}

func discoverRunnerRootsFromGlobs() []RunnerRoot {
	var roots []RunnerRoot
	for _, pattern := range defaultRunnerRootGlobs {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, m := range matches {
			info, err := os.Stat(m)
			if err != nil || !info.IsDir() {
				continue
			}
			roots = append(roots, RunnerRoot{Name: filepath.Base(m), Path: m})
		}
	}
	return roots
}
