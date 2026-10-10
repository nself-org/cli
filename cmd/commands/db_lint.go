package commands

// Purpose: runDBLint, the RunE for "nself db lint". Inputs are the cobra
// command/args and the loaded project config; outputs are printed lint
// findings or an error.
// Constraints: split out of db.go (CLI-R12) as a pure move, no behavior change.

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/tenant"

	"github.com/spf13/cobra"
)

func runDBLint(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	rlsFlag, _ := cmd.Flags().GetBool("rls")
	metricFlag, _ := cmd.Flags().GetBool("metric")
	matrixFlag, _ := cmd.Flags().GetBool("matrix")
	remediateFlag, _ := cmd.Flags().GetBool("remediate")
	formatFlag, _ := cmd.Flags().GetString("format")

	// Legacy path: no --rls flag uses the original tenant_id-only check.
	if !rlsFlag && !metricFlag {
		results, err := tenant.LintRLS(cmd.Context(), cfg)
		if err != nil {
			return fmt.Errorf("db lint: %w", err)
		}
		if jsonEnvelopeOn(cmd) {
			if results == nil {
				results = []tenant.LintResult{}
			}
			return emitEnv(cmd, dbLintData{Tables: results}, false)
		}
		if len(results) == 0 {
			fmt.Println("No tenant-scoped tables found.")
			return nil
		}
		if formatFlag == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(results)
		}
		failures := 0
		for _, r := range results {
			status := "PASS"
			if !r.Pass {
				status = "FAIL"
				failures++
			}
			fmt.Printf("[%s] %s.%s — %s\n", status, r.Schema, r.Table, r.Message)
		}
		if failures > 0 {
			return fmt.Errorf("%d table(s) with tenant_id missing RLS policies", failures)
		}
		fmt.Printf("All %d tenant-scoped table(s) have RLS policies.\n", len(results))
		return nil
	}

	// Full RLS audit path.
	allowlist := loadAllowlist(cfg)
	report, err := tenant.LintRLSFull(cmd.Context(), cfg, allowlist)
	if err != nil {
		return fmt.Errorf("db lint --rls: %w", err)
	}

	// --metric: emit Prometheus text line and exit.
	if metricFlag {
		count := tenant.DisabledTableCount(report)
		fmt.Printf("# HELP nself_rls_disabled_tables Number of user-data tables without RLS.\n")
		fmt.Printf("# TYPE nself_rls_disabled_tables gauge\n")
		fmt.Printf("nself_rls_disabled_tables %d\n", count)
		return nil
	}

	if jsonEnvelopeOn(cmd) {
		if !matrixFlag {
			report.CoverageMatrix = nil
		}
		return emitEnv(cmd, dbLintData{Tables: []tenant.LintResult{}, Report: report}, false)
	}
	if formatFlag == "json" {
		if !matrixFlag {
			report.CoverageMatrix = nil
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	// Table output.
	fmt.Printf("RLS Audit: %d tables checked, %d enabled, %d disabled, %d allowlisted, %d violations\n\n",
		report.TotalTables, report.RLSEnabled, report.RLSDisabled, report.Allowlisted, report.Violations)

	for _, r := range report.Tables {
		status := "PASS"
		if !r.Pass {
			status = "FAIL"
		}
		if r.Allowlisted {
			status = "SKIP"
		}
		fmt.Printf("[%s] %s.%s (policies: %d) — %s\n", status, r.Schema, r.Table, r.PolicyCount, r.Message)
	}

	// Coverage matrix.
	if matrixFlag && len(report.CoverageMatrix) > 0 {
		fmt.Printf("\n--- Coverage Matrix (table x role) ---\n")
		roles := []string{"anonymous", "user", "tenant_member", "tenant_admin", "admin", "service"}
		fmt.Printf("%-40s", "TABLE")
		for _, role := range roles {
			fmt.Printf(" %-16s", role)
		}
		fmt.Println()
		for _, entry := range report.CoverageMatrix {
			fmt.Printf("%-40s", entry.Schema+"."+entry.Table)
			for _, role := range roles {
				pol := entry.Roles[role]
				if pol == "" {
					pol = "none"
				}
				if len(pol) > 15 {
					pol = pol[:15]
				}
				fmt.Printf(" %-16s", pol)
			}
			fmt.Println()
		}
	}

	// Remediation SQL.
	if remediateFlag {
		sql := tenant.GenerateRemediationSQL(report)
		if sql != "" {
			fmt.Printf("\n--- Remediation SQL ---\n%s", sql)
		} else {
			fmt.Println("\nNo remediation needed.")
		}
	}

	if report.Violations > 0 {
		return fmt.Errorf("%d table(s) failing RLS audit", report.Violations)
	}
	return nil
}
