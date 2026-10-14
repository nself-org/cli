package commands

// start_cmd.go — cobra command registration for `nself start`.
// Purpose: declares startCmd (flags, usage text) and registers it on
//          RootCmd. Split out of start.go (T-P6-E2-W1-S1-T3) for 300-line
//          compliance. Also holds runStart, the `start --json` wrapper around
//          runStartFlow (start.go), and the StartResult assembly (P7-SURF-10).
// Inputs:  none (invoked once at package init via init()).
// Outputs: none — side effect is RootCmd.AddCommand(startCmd).
// Constraints: same flags/usage text/defaults; human output unchanged.

import (
	"context"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/health"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"up"},
	// runStart ignores positional arguments entirely — it always boots the
	// whole stack. Accepting them silently is how `nself admin start` ended
	// up shelling out to `nself start admin` and booting everything instead
	// of one service. Reject them loudly rather than appearing to scope.
	Args:  cobra.NoArgs,
	Short: "Boot your nSelf stack",
	Long: `Boot the nSelf stack with health checks and automatic database initialization.

Executes the startup sequence:
  1. Validate docker-compose.yml exists
  2. Load environment configuration
  3. Check port availability
  4. Start PostgreSQL
  5. Initialize database (schemas, extensions)
  6. Start remaining services
  7. Run health checks on all services
  8. Display service URLs`,
	// P7-LIVE-09: compare the stack with nself.lock.yaml before anything runs.
	PreRunE: startLockPreRun,
	RunE:    runStart,
}

// runStart boots the stack (runStartFlow); in JSON mode it ends with the
// lifecycle result (the last line of a json-stream under --watch).
func runStart(cmd *cobra.Command, args []string) error {
	began := time.Now()
	var out startOutcome
	if err := runStartFlow(cmd, args, &out); err != nil {
		return err
	}
	if !lifecycleJSON(cmd) {
		return nil
	}
	return emitLifecycle(cmd, "start", startResult(cmd, out, time.Since(began).Milliseconds()))
}

func init() {
	f := startCmd.Flags()
	f.BoolP("verbose", "v", false, "Show detailed Docker output")
	f.BoolP("debug", "d", false, "Show debug information")
	f.Bool("skip-health-checks", false, "Skip health validation after startup")
	f.Int("timeout", 120, "Health check timeout in seconds (range: 30-600)")
	f.Bool("fresh", false, "Force recreate all containers")
	f.Bool("force-recreate", false, "Alias for --fresh")
	f.Bool("clean-start", false, "Remove all containers before starting")
	f.Bool("quick", false, "Quick start (timeout=30, required=60%)")
	f.Bool("skip-port-check", false, "Skip port availability check")
	f.Bool("skip-build", false, "Skip automatic rebuild detection")
	f.Bool("skip-plugins", false, "Start base stack only, skip all plugin compose files")
	f.Bool("watch", false, "Enable health auto-restart: poll services and restart unhealthy containers")
	f.Bool("quiet", false, "Suppress progress output (for CI; preserves --json output)")
	f.Bool("allow-legacy", false, "Bypass v0.9 artifact check and proceed with WARNING (not recommended)")
	f.Bool("embedded-pg", false, "Boot PostgreSQL via embedded pglite/wasmtime — no Docker postgres container required; pgvector included")
	f.Bool("frozen", false, "Fail with E455 unless the CLI-owned service images (not CS_N or plugin-fragment services), plugin versions and checksums, and CLI minor match nself.lock.yaml")
	f.Bool("skip-db-init", false, "Skip database migrations and seed; bring up Postgres+Hasura+hasura-auth only. Intended for CI/E2E environments.")
	f.String("profile", "", `Service profile passed to an automatic rebuild when the compose file is stale.
  app (default) — full service set.
  ops           — observability + CI server (postgres, hasura, auth, nginx, monitoring).
Overrides NSELF_PROFILE env var. Has no effect when --skip-build is set.`)

	RootCmd.AddCommand(startCmd)

}

// startOutcome is what a finished `start` hands to its result: the project and
// the last health report of the run (nil when health checks were skipped).
type startOutcome struct {
	cfg    *config.Config
	dir    string
	report *health.HealthReport
}

// startResult builds the StartResult of a finished start. Without a report from
// the run (health checks skipped, CI gate) it asks the health checker once.
func startResult(cmd *cobra.Command, out startOutcome, elapsedMS int64) StartResult {
	res := StartResult{Services: []StartService{}, URLs: []string{}, DurationMS: elapsedMS}
	if out.cfg == nil {
		return res
	}
	report := out.report
	if report == nil {
		report, _ = health.RunAllChecks(commandContext(cmd), out.cfg, out.dir)
	}
	if report != nil {
		for _, r := range report.Results {
			status, healthState := serviceState(r.Status)
			res.Services = append(res.Services, StartService{Name: r.Service, Status: status, Health: healthState})
		}
	}
	for _, line := range stackURLs(out.cfg) {
		if f := strings.Fields(line); len(f) > 0 && strings.Contains(f[len(f)-1], "://") {
			res.URLs = append(res.URLs, f[len(f)-1])
		}
	}
	return res
}

// commandContext is the command's context, or Background for a command that
// was not run through Execute.
func commandContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

// serviceState splits a health.HealthResult status into the container state and
// the healthcheck result. "running" is a container without a healthcheck.
func serviceState(raw string) (status, healthState string) {
	switch raw {
	case "healthy", "unhealthy", "starting":
		return "running", raw
	case "running":
		return "running", "none"
	case "not_found":
		return "missing", "none"
	}
	return "unknown", "none"
}
