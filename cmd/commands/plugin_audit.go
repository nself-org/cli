package commands

// plugin_audit.go — S1.T08: nself plugin audit-tables
//
// Reads row counts per np_* table per source_account_id and checks Hasura
// metadata for source_account_id row filters.
//
// Exit codes:
//   0 — all tables compliant (have source_account_id AND Hasura filter)
//   1 — one or more tables missing source_account_id column
//   2 — one or more tables missing Hasura select_permissions filter
//       (only reached when no exit-1 violations exist)
//
// This command is read-only; it never modifies DB state.
// Security-Always-Free Doctrine: runs without a license key.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/nself-org/cli/internal/audit"
	"github.com/spf13/cobra"

	"github.com/nself-org/cli/internal/errs"
)

var pluginAuditTablesCmd = &cobra.Command{
	Use:   "audit-tables",
	Short: "Audit np_* table row counts and multi-tenant isolation compliance",
	Long: `Read row counts for every np_* table in the public schema, grouped by
source_account_id, and verify Hasura metadata contains source_account_id
row filters for each table.

Exit codes:
  0 — all tables compliant
  1 — one or more tables missing source_account_id column
  2 — one or more tables missing Hasura select_permissions filter

Requires NSELF_DB_URL (or DATABASE_URL) to be set.

Examples:
  nself plugin audit-tables
  nself plugin audit-tables --json
  nself plugin audit-tables --table np_chat_messages
  nself plugin audit-tables --json > baseline.json
  diff baseline.json <(nself plugin audit-tables --json)`,
	RunE: runPluginAuditTables,
}

func init() {
	pluginAuditTablesCmd.Flags().Bool("json", false, "Output as JSON instead of a table")
	pluginAuditTablesCmd.Flags().String("table", "", "Audit a single table by name (default: all np_* tables)")
	pluginCmd.AddCommand(pluginAuditTablesCmd)
}

func runPluginAuditTables(cmd *cobra.Command, _ []string) error {
	asJSON, _ := cmd.Flags().GetBool("json")
	filterTable, _ := cmd.Flags().GetString("table")

	cfg := audit.AuditConfig{FilterTable: filterTable}
	report, err := audit.RunTableAuditWithConfig(context.Background(), cfg)
	if err != nil {
		return fmt.Errorf("audit-tables: %w", err)
	}

	if jsonEnvelopeOn(cmd) {
		if err := emitEnv(cmd, report, true); err != nil { // P7-SURF-13
			return err
		}
	} else if asJSON {
		out, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal audit report: %w", err)
		}
		fmt.Println(string(out))
	} else {
		printAuditTable(report)
	}

	// Determine exit code: check for violations in priority order. The report
	// is already printed, so these are silent status-only exits.
	for _, t := range report.Tables {
		if !t.HasSourceAccountID {
			return errs.Exit(1)
		}
	}
	for _, t := range report.Tables {
		if t.HasHasuraFilter == audit.HasuraFilterMissing {
			return errs.Exit(2)
		}
	}
	return nil
}

// printAuditTable renders the audit report as a human-readable table.
func printAuditTable(report *audit.AuditReport) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TABLE\tSOURCE_ACCOUNT_ID\tHASURA_FILTER\tROW_COUNT\tWARNING")
	for _, t := range report.Tables {
		hasCol := "yes"
		if !t.HasSourceAccountID {
			hasCol = "NO"
		}
		filter := string(t.HasHasuraFilter)
		warning := t.Warning
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n",
			t.Table, hasCol, filter, t.TotalRows, warning)
	}
	_ = tw.Flush()
	fmt.Printf("\nAudit completed at %s (%d tables)\n", report.AuditAt, len(report.Tables))
}
