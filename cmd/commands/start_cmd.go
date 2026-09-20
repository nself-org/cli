package commands

// start_cmd.go — cobra command registration for `nself start`.
// Purpose: declares startCmd (flags, usage text) and registers it on
//          RootCmd. Split out of start.go (T-P6-E2-W1-S1-T3) for 300-line
//          compliance.
// Inputs:  none (invoked once at package init via init()).
// Outputs: none — side effect is RootCmd.AddCommand(startCmd).
// Constraints: pure move, same flags/usage text/defaults, no behavior change.

import "github.com/spf13/cobra"

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
	RunE: runStart,
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
	f.Bool("skip-db-init", false, "Skip database migrations and seed; bring up Postgres+Hasura+hasura-auth only. Intended for CI/E2E environments.")
	f.String("profile", "", `Service profile passed to an automatic rebuild when the compose file is stale.
  app (default) — full service set.
  ops           — observability + CI server (postgres, hasura, auth, nginx, monitoring).
Overrides NSELF_PROFILE env var. Has no effect when --skip-build is set.`)

	RootCmd.AddCommand(startCmd)

}
