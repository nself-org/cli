package admin

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
)

// adminPort returns the admin UI port from env or falls back to 3021.
func adminPort() string {
	if p := os.Getenv("NSELF_ADMIN_PORT"); p != "" {
		return p
	}
	if p := os.Getenv("ADMIN_PORT"); p != "" {
		return p
	}
	return "3021"
}

// adminComposeContext resolves the project directory and compose files used to
// bring up the admin service on its own. Mirrors how the start path resolves
// them, but deliberately does not import the start pipeline.
func adminComposeContext() (string, []string) {
	projectDir, err := os.Getwd()
	if err != nil {
		projectDir = "."
	}
	return projectDir, nil
}

// adminContainerID returns the docker container name for the admin service.
func (r runner) adminContainerID() string {
	cfg, _, err := r.d.LoadHealthConfig()
	if err != nil || cfg.ProjectName == "" {
		return "nself_admin"
	}
	return fmt.Sprintf("%s_admin", cfg.ProjectName)
}

func rootCommand(r runner) *cobra.Command {
	return &cobra.Command{
		Use:   "admin [subcommand]",
		Short: "Manage the nSelf Admin dashboard",
		Long: `Open, start, stop, inspect logs for, or health-check the nSelf Admin UI.

With no subcommand, opens the Admin dashboard (http://localhost:3021) in your
default browser.

Subcommands:
  start   Start the Admin service (enables + builds + boots if not running)
  stop    Stop the Admin container gracefully
  logs    Tail Admin container logs
  health  Check Admin liveness (HTTP probe on /health)`,
		RunE: r.runAdmin,
	}
}

// runAdmin opens the Admin UI in the default browser (default action, no subcommand).
func (r runner) runAdmin(cmd *cobra.Command, args []string) error {
	port := adminPort()
	adminURL := "http://localhost:" + port

	// Skip the browser launch in test / CI / headless contexts. Launching
	// Safari or xdg-open here during `go test` caused orphan GUI processes
	// that pushed the macos-14 CI job past its 10-minute timeout.
	if !r.d.ShouldOpenBrowser() {
		fmt.Printf("Admin UI: %s\n", adminURL)
		return nil
	}

	openCmd := r.d.OpenBrowserCmd(cmd.Context(), adminURL)
	if openCmd == nil {
		fmt.Printf("Admin UI: %s\n", adminURL)
		return nil
	}

	if err := openCmd.Start(); err != nil {
		// Graceful fallback — headless server or command not found.
		fmt.Printf("Admin UI: %s\n", adminURL)
		return nil
	}

	fmt.Printf("Opening %s in your browser...\n", adminURL)
	return nil
}
