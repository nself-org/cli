package commands

// Purpose: Implements `nself service list` (per-service enabled/status/port
// table) and the shared service catalog printer used by both `service list
// --core` and the docs-generation path — the same internal/compose catalog
// data the generated wiki page is built from. Split out of service.go
// (CLI-R12) to separate the listing/catalog display logic from the cobra
// command definitions and the add/upgrade/enable/disable/configure/
// lifecycle handlers in the other service_*.go files.
// Inputs: the cobra.Command + args for `service list` (env/json/core flags).
// Outputs: a printed table or JSON array of serviceStatusRow / catalogRow
// values.
// Constraints: pure move — no behavior changes. printServiceCatalog must
// keep sourcing from compose.ServiceCatalog() so the CLI and the generated
// wiki page cannot disagree.

import (
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// --- run functions ---

// serviceStatusRow holds data for a single row in the service list output.
type serviceStatusRow struct {
	Service string `json:"service"`
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"`
	Port    string `json:"port"`
	EnvVar  string `json:"env_var"`
}

func runServiceList(cmd *cobra.Command, args []string) error {
	envFlag, _ := cmd.Flags().GetString("env")
	jsonOut, _ := cmd.Flags().GetBool("json")
	coreOnly, _ := cmd.Flags().GetBool("core")

	// --core answers "what does a minimal nSelf stack contain?" from the
	// compose catalog rather than from the project's .env, so it works before
	// `nself init` and gives the docs a machine-readable source (CLI-R07).
	if coreOnly {
		if jsonEnvelopeOn(cmd) {
			return emitEnv(cmd, newServiceListData(nil, serviceCatalogRows()), true) // P7-SURF-13
		}
		return printServiceCatalog(jsonOut)
	}

	envFile, err := resolveEnvFile(envFlag)
	if err != nil {
		return err
	}

	values, err := readEnvValues(envFile)
	if err != nil {
		return err
	}

	// Emit deprecation warnings for legacy env vars.
	if v, ok := values["MAIL_ENABLED"]; ok && strings.ToLower(strings.TrimSpace(v)) == "true" {
		fmt.Fprintln(os.Stderr, "DEPRECATED: MAIL_ENABLED is deprecated; use EMAIL_ENABLED (or MAILPIT_ENABLED)")
	}
	if v, ok := values["MLFLOW_ENABLED"]; ok && strings.ToLower(strings.TrimSpace(v)) == "true" {
		fmt.Fprintln(os.Stderr, "DEPRECATED: MLFLOW_ENABLED is no longer an optional service. Run:")
		fmt.Fprintln(os.Stderr, "  nself plugin install mlflow")
		fmt.Fprintln(os.Stderr, "Then remove MLFLOW_ENABLED from your .env")
	}

	rows := make([]serviceStatusRow, 0, len(optionalServices()))
	for _, svc := range optionalServices() {
		enabled := false
		if v, ok := values[svc.EnvVar]; ok && strings.ToLower(strings.TrimSpace(v)) == "true" {
			enabled = true
		}
		status := "disabled"
		if enabled {
			status = "enabled"
		}
		enabledStr := "no"
		if enabled {
			enabledStr = "yes"
		}

		portStr := fmt.Sprintf("%d", svc.Port)
		if svc.Name == "monitoring" {
			portStr = "multi"
		} else if svc.Port == 0 {
			portStr = "N/A"
		}

		_ = enabledStr // used in table below
		rows = append(rows, serviceStatusRow{
			Service: svc.Name,
			Enabled: enabled,
			Status:  status,
			Port:    portStr,
			EnvVar:  svc.EnvVar,
		})
	}

	if jsonEnvelopeOn(cmd) {
		return emitEnv(cmd, newServiceListData(rows, nil), true) // P7-SURF-13
	}
	if jsonOut {
		return ui.PrintJSON(rows)
	}

	tbl := ui.NewTable("SERVICE", "STATUS", "ENABLED", "PORT", "ENV_VAR")
	for _, r := range rows {
		enabledStr := "no"
		if r.Enabled {
			enabledStr = "yes"
		}
		tbl.AddRow(r.Service, r.Status, enabledStr, r.Port, r.EnvVar)
	}
	tbl.Render()

	return nil
}

type catalogRow struct {
	Service      string `json:"service"`
	Tier         string `json:"tier"`
	Purpose      string `json:"purpose"`
	EnableEnv    string `json:"enable_env,omitempty"`
	VersionEnv   string `json:"version_env"`
	DefaultImage string `json:"default_image"`
}

// printServiceCatalog renders the required/optional service catalog. It is the
// published form of internal/compose's catalog — the same data the generated
// wiki page is built from, so the CLI and the docs cannot disagree.
func printServiceCatalog(jsonOut bool) error {
	rows := serviceCatalogRows()

	if jsonOut {
		return ui.PrintJSON(rows)
	}

	tbl := ui.NewTable("SERVICE", "TIER", "ENABLE_ENV", "DEFAULT_IMAGE", "PURPOSE")
	for _, r := range rows {
		enable := r.EnableEnv
		if enable == "" {
			enable = "— (always on)"
		}
		tbl.AddRow(r.Service, r.Tier, enable, r.DefaultImage, r.Purpose)
	}
	tbl.Render()
	return nil
}

// serviceCatalogRows lists the compose catalog as catalog rows.
func serviceCatalogRows() []catalogRow {
	entries := compose.ServiceCatalog()
	rows := make([]catalogRow, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, catalogRow{
			Service:      e.Name,
			Tier:         string(e.Tier),
			Purpose:      e.Purpose,
			EnableEnv:    e.EnableEnv,
			VersionEnv:   e.VersionEnv,
			DefaultImage: e.DefaultImage,
		})
	}
	return rows
}
