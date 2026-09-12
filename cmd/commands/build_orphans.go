package commands

// build_orphans.go — G-014: `nself build` orphan-container detection
// (always on) and removal (opt-in via --remove-orphans).
//
// WHY: nself build only ever generated files; it never looked at the live
// daemon, so a service dropped from the generated compose (removed from
// nself.yaml, an uninstalled plugin, a rename) left its container running
// forever with no service definition, no nginx vhost, and no traffic —
// unreported. Measured live: nself-claw/notify/mux/cron survived an earlier
// build this way; two had been dead a full month (DB DNS failure) before
// anyone noticed, because a second bug made their healthchecks permanently
// meaningless too (see internal/doctor/deep_docker_healthcheck.go).
//
// Purpose: report every such container prominently after every build, and
// remove them only when the operator opted in.
// Inputs:  workdir (the project root just built), result (the BuildResult
//          from build.Build), removeOrphans (--remove-orphans), quiet.
// Outputs: none — prints to stdout/stderr; never returns an error, because
//          orphan detection is advisory and must never fail `nself build`
//          itself (a Docker-less CI image must still get a successful build).
// Constraints: project-scoped via internal/docker.DetectOrphans's compose
//              project label filter (see that file's scoping note) — never
//              touches a container from another project or a non-compose
//              container.

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/ui"
)

// detectOrphansForProject resolves the effective compose file set for
// workdir (base + plugin fragments, falling back to fallbackComposeFile
// when the manifest is missing) and returns the containers orphaned against
// it. Shared by `nself build` (reportAndHandleOrphans, below) and `nself
// status` (see status.go), which both need the same detection but differ on
// what to do with the result — build offers --remove-orphans, status is
// read-only. Returns (nil, nil) whenever detection is inconclusive (compose
// files unreadable, docker unreachable); callers must treat that as "nothing
// to report", never as a hard failure of their own command.
func detectOrphansForProject(ctx context.Context, workdir, projectName, fallbackComposeFile string) []docker.OrphanContainer {
	composeFiles, err := build.ReadComposeManifest(workdir)
	if err != nil || len(composeFiles) == 0 {
		composeFiles = []string{fallbackComposeFile}
	}

	defined, err := docker.ComposeServiceNames(composeFiles)
	if err != nil {
		slog.Debug("G-014 orphan detection: could not read compose files", "err", err)
		return nil
	}

	orphans, err := docker.DetectOrphans(ctx, projectName, defined)
	if err != nil {
		slog.Debug("G-014 orphan detection: docker unavailable, skipping", "err", err)
		return nil
	}
	return orphans
}

// reportAndHandleOrphans runs orphan detection for the project just built
// and, if --remove-orphans was passed, removes what it finds. Best-effort
// throughout: any docker-level failure (daemon unreachable, docker not
// installed) is logged at debug level and swallowed so `nself build` itself
// never fails because of it.
func reportAndHandleOrphans(workdir string, result *build.BuildResult, removeOrphans, quiet bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	orphans := detectOrphansForProject(ctx, workdir, result.ProjectName, result.ComposeFile)
	if len(orphans) == 0 {
		return
	}

	if !quiet {
		printOrphanWarning(orphans)
	}

	if !removeOrphans {
		if !quiet {
			ui.Warn(fmt.Sprintf("Run 'nself build --remove-orphans' to remove the %d container(s) above.", len(orphans)))
		}
		return
	}

	removeErrs := docker.RemoveOrphans(ctx, orphans)
	if quiet {
		return
	}
	removed := len(orphans) - len(removeErrs)
	if removed > 0 {
		ui.Success(fmt.Sprintf("Removed %d orphaned container(s)", removed))
	}
	for _, e := range removeErrs {
		ui.Warn(e.Error())
	}
}

// printOrphanWarning prints the prominent, always-on orphan report: one
// line per container so an operator scanning build output cannot miss it.
func printOrphanWarning(orphans []docker.OrphanContainer) {
	ui.Warn(fmt.Sprintf("%d orphaned container(s) found — running but with no matching service in the generated compose:", len(orphans)))
	for _, o := range orphans {
		service := o.Service
		if service == "" {
			service = "(none)"
		}
		fmt.Fprintf(os.Stderr, "  %-30s service=%-12s state=%s\n", o.Name, service, o.State)
	}
}

// printOrphanStatusHint runs the same G-014 orphan detection for `nself
// status` (read-only — status never removes anything; that stays build's
// --remove-orphans opt-in) and, if any are found, prints a warning block
// pointing at the fix. Best-effort and silent on any docker-level failure —
// status must never fail or slow down because of this.
func printOrphanStatusHint(ctx context.Context, workdir, projectName, composeFile string) {
	orphans := detectOrphansForProject(ctx, workdir, projectName, composeFile)
	if len(orphans) == 0 {
		return
	}
	fmt.Println()
	printOrphanWarning(orphans)
	ui.Warn("Run 'nself build --remove-orphans' to remove them.")
}
