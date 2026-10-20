package commands

// Purpose: the "nself controller start/stop/status" subcommands. Inputs are
// the cobra command/args; outputs are printed controller state or an error.
// Constraints: split out of controller.go (CLI-R12) as a pure move, no behavior change.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var controllerCmd = &cobra.Command{
	Use:   "controller",
	Short: "Manage the multi-tenant master controller (nCloud)",
	Long: `Commands for the nCloud multi-tenant master controller.

The controller manages N isolated nSelf project instances on a single server.
Requires NSELF_FLAG_MULTI_TENANT_CONTROLLER=true.

Subcommands:
  start   Start the controller daemon
  stop    Stop the controller daemon
  status  Show all projects and resource usage
  init    Initialize the np_tenant_controller Postgres schema`,
}

var controllerStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the controller daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !assertControllerEnabled(cmd) {
			return nil
		}
		// The controller daemon is managed as a systemd service or nself service unit.
		// Delegate to the service manager.
		c := exec.Command("systemctl", "start", "nself-controller")
		c.Stdout = cmd.OutOrStdout()
		c.Stderr = cmd.ErrOrStderr()
		if err := c.Run(); err != nil {
			// Fallback for non-systemd environments.
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Hint: run the controller binary directly:")
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  NSELF_FLAG_MULTI_TENANT_CONTROLLER=true nself-tenant-controller &")
		}
		return nil
	},
}

var controllerStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the controller daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !assertControllerEnabled(cmd) {
			return nil
		}
		c := exec.Command("systemctl", "stop", "nself-controller")
		c.Stdout = cmd.OutOrStdout()
		c.Stderr = cmd.ErrOrStderr()
		_ = c.Run()
		return nil
	},
}

var controllerStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show all projects and resource usage",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !assertControllerEnabled(cmd) {
			return nil
		}
		body, code, err := doControllerRequest("GET", "/controller/status", nil)
		if err != nil {
			return fmt.Errorf("controller unreachable: %w\nHint: run 'nself controller start'", err)
		}
		if code != http.StatusOK {
			return fmt.Errorf("controller status %d: %s", code, body)
		}

		var cs struct {
			Projects []struct {
				Project struct {
					Slug      string `json:"slug"`
					Domain    string `json:"domain"`
					Status    string `json:"status"`
					CreatedAt string `json:"created_at"`
				} `json:"project"`
				ConnectionCount int   `json:"connection_count"`
				SchemaBytes     int64 `json:"schema_bytes_approx"`
				Healthy         bool  `json:"healthy"`
			} `json:"projects"`
			TotalActive    int     `json:"total_active"`
			MaxProjects    int     `json:"max_projects"`
			UtilizationPct float64 `json:"utilization_pct"`
		}
		if err := json.Unmarshal(body, &cs); err != nil {
			return fmt.Errorf("parse response: %w", err)
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintf(w, "SLUG\tDOMAIN\tSTATUS\tCONNS\tSIZE\tHEALTHY\n")
		for _, p := range cs.Projects {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%v\n",
				p.Project.Slug,
				p.Project.Domain,
				p.Project.Status,
				p.ConnectionCount,
				formatBytes(p.SchemaBytes),
				p.Healthy,
			)
		}
		_ = w.Flush()
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n%d / %d projects active (%.0f%% utilized)\n",
			cs.TotalActive, cs.MaxProjects, cs.UtilizationPct)
		return nil
	},
}

// ---- project commands ----
