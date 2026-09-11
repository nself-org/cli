package maintenance

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// DiskUsage holds before/after disk utilisation for a cleanup run.
type DiskUsage struct {
	// UsedPercent is the percentage of the disk that is in use (0-100).
	UsedPercent int
	// TotalGB is the total disk capacity in gigabytes.
	TotalGB float64
	// UsedGB is the used disk space in gigabytes.
	UsedGB float64
	// FreeGB is the free disk space in gigabytes.
	FreeGB float64
}

// reclaimTier labels which safety tier a reclaim (or would-be reclaim) belongs to.
// See disk.go's DiskCleanupWithOptions for how each tier is gated.
type reclaimTier string

const (
	// tierAlways reclaims are safe unconditionally: busy or idle, under pressure or
	// not. Nothing a running job reads lives here (docker dangling images/build
	// cache/anonymous volumes, old compressed logs, journald history).
	tierAlways reclaimTier = "always"
	// tierIdlePerRunner reclaims (runner job workspaces) only run for a runner root
	// that is individually idle. Never escalated by disk pressure — deleting an
	// in-progress job's own checkout would break that job outright, the same failure
	// mode as deleting _actions.
	tierIdlePerRunner reclaimTier = "idle-per-runner"
	// tierIdleShared reclaims (regenerable package/module caches shared across
	// runners) prefer global idle, but escalate to run regardless of busy state once
	// disk usage crosses the pressure threshold — a full disk fails every job on the
	// box, so waiting for idle at that point is strictly worse.
	tierIdleShared reclaimTier = "idle-shared"
)

// ReclaimEntry records one thing DiskCleanup removed (or, in dry-run mode, would
// remove).
type ReclaimEntry struct {
	Path  string
	Bytes int64
	Tier  reclaimTier
}

// SkipEntry records one thing DiskCleanup deliberately left alone, and why. This is
// what makes a `disk-cleanup` timer run diagnosable after the fact — "ran and found
// nothing to do" and "refused to touch anything because everything was busy" used to
// be indistinguishable in the log.
type SkipEntry struct {
	Path   string
	Reason string
}

// CleanupResult summarises what disk-cleanup did.
type CleanupResult struct {
	Before DiskUsage
	After  DiskUsage

	// DryRun is true when this result came from a dry-run — Reclaimed lists what
	// WOULD have been removed and Bytes are estimates; nothing was actually deleted.
	DryRun bool

	// BytesReclaimed is the total size of everything actually removed (0 for a
	// dry-run's real disk impact, but Reclaimed still lists the would-be total).
	BytesReclaimed int64
	Reclaimed      []ReclaimEntry
	Skipped        []SkipEntry

	DockerPruneOut   string
	LogRotationOut   string
	JournalVacuumOut string

	Errors []error
}

// runCommand executes a command and returns combined stdout+stderr output and any error.
func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		return trimmed, fmt.Errorf("%w\n%s", err, trimmed)
	}
	return trimmed, nil
}

// hexVolumeNameRe matches Docker's auto-generated anonymous volume names (64 lowercase
// hex characters). Named volumes such as "ntask_data" never match.
var hexVolumeNameRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// filterAnonymousVolumes returns only the entries in names that look like Docker
// anonymous volume IDs. It is the safety gate behind dockerReclaim's volume cleanup:
// `docker volume prune` alone removes ALL unused volumes, named or not, so anything
// that isn't hex-named is never even considered for removal here.
func filterAnonymousVolumes(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n != "" && hexVolumeNameRe.MatchString(n) {
			out = append(out, n)
		}
	}
	return out
}

// dockerReclaim replaces the old `docker system prune -af --volumes=false`, which is
// unsafe on any host running an nself stack: `-a` removes ALL unused images —
// including ones only a *stopped* stack container still depends on. A routine run of
// that exact command previously deleted the images backing ntask's postgres/redis
// containers and caused a 3-day Hasura outage, recovered only via `nself restart
// <svc>` plus an nginx reload.
//
// This version can only ever remove:
//   - dangling (untagged, "<none>") images — never referenced by any container,
//     stopped or running;
//   - the build cache — regenerable, holds no runtime data;
//   - anonymous dangling volumes (64-hex generated names, via filterAnonymousVolumes)
//     — never a named volume like "*_data" that a stack service mounts.
//
// dockerReclaimFunc is a var (not a plain function call) so tests can stub out real
// docker invocations entirely — disk-cleanup tests must never shell out to a real
// docker daemon.
var dockerReclaimFunc = dockerReclaim

func dockerReclaim(dryRun bool) (out string, errs []error) {
	var b strings.Builder

	if dryRun {
		imgOut, _ := runCommand("docker", "images", "-f", "dangling=true", "-q")
		imgCount := len(nonEmptyLines(imgOut))
		fmt.Fprintf(&b, "would remove %d dangling image(s)\n", imgCount)

		volOut, _ := runCommand("docker", "volume", "ls", "-q", "-f", "dangling=true")
		anon := filterAnonymousVolumes(strings.Split(volOut, "\n"))
		fmt.Fprintf(&b, "would remove %d anonymous dangling volume(s)\n", len(anon))
		fmt.Fprintf(&b, "would prune docker build cache\n")
		return b.String(), nil
	}

	imgOut, imgErr := runCommand("docker", "image", "prune", "-f")
	b.WriteString(imgOut)
	b.WriteString("\n")
	if imgErr != nil {
		errs = append(errs, fmt.Errorf("docker image prune: %w", imgErr))
	}

	buildOut, buildErr := runCommand("docker", "builder", "prune", "-f")
	b.WriteString(buildOut)
	b.WriteString("\n")
	if buildErr != nil {
		errs = append(errs, fmt.Errorf("docker builder prune: %w", buildErr))
	}

	volListOut, volListErr := runCommand("docker", "volume", "ls", "-q", "-f", "dangling=true")
	if volListErr != nil {
		errs = append(errs, fmt.Errorf("docker volume ls: %w", volListErr))
	} else {
		removed := 0
		for _, name := range filterAnonymousVolumes(strings.Split(volListOut, "\n")) {
			if _, err := runCommand("docker", "volume", "rm", name); err != nil {
				errs = append(errs, fmt.Errorf("docker volume rm %s: %w", name, err))
				continue
			}
			removed++
		}
		fmt.Fprintf(&b, "removed %d anonymous dangling volume(s)\n", removed)
	}

	return b.String(), errs
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}
