package commands

// Purpose: the "nself doctor" subcommand registration (init), the legacy
// runDoctorCheckLegacy path, and the "install-check" JSON report types plus
// runInstallCheck/printInstallCheckJSON. Inputs are the cobra command/args;
// outputs are printed diagnostics or JSON, or an error.
// Constraints: split out of doctor.go (CLI-R12) as a pure move, no behavior change.

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/migration"
	"github.com/nself-org/cli/internal/onboarding"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/ui"
)

func init() {
	doctorCmd.Flags().Bool("verbose", false, "Detailed diagnostics")
	doctorCmd.Flags().Bool("full", false, "Run all checks including network and memory (slower)")
	doctorCmd.Flags().Bool("deep", false, "Alias for --full (run all checks)")
	doctorCmd.Flags().Bool("quick", false, "Fast check: infrastructure, disk, ports, config, plugins, license, container health only (the default check set; explicit opt-in overrides --full/--deep)")
	doctorCmd.Flags().Bool("fix", false, "Auto-fix safe issues")
	doctorCmd.Flags().Bool("plan", false, "With --fix, show generated-file repair plan without writing")
	doctorCmd.Flags().Bool("force", false, "With --fix, overwrite hand-edited generated files")
	doctorCmd.Flags().Bool("json", false, "JSON output")
	doctorCmd.Flags().String("section", "", "Run only a specific section (system, core, backups, license, plugins, monitoring, security)")
	doctorCmd.Flags().StringSlice("skip", nil, "Skip specific check sections")
	doctorCmd.Flags().Bool("alerts", false, "Check monitoring alert rules are loaded")
	doctorCmd.Flags().String("format", "", "Output format: json, text (default text)")
	doctorCmd.Flags().String("only", "", "Run only a specific subsystem check (host, docker, postgres, hasura, nginx, ssl, ping, plugins, license, monitoring, backups, security)")
	// AI wizard flags (T-05-01, T-05-02)
	doctorCmd.Flags().Bool("ai", false, "Run the AI first-run wizard (install Ollama, setup Gemini pool, verify)")
	doctorCmd.Flags().Bool("yes", false, "Non-interactive mode: accept all defaults (for CI/scripts)")
	doctorCmd.Flags().Bool("skip-ollama", false, "Skip local Ollama installation step")
	doctorCmd.Flags().Bool("skip-pool", false, "Skip Gemini pool setup step")
	doctorCmd.Flags().Bool("headless", false, "Print OAuth URL instead of opening browser (for SSH/headless servers)")
	doctorCmd.Flags().Bool("check-legacy", false, "Scan host for v0.9 stale paths (global scan, not per-project)")
	doctorCmd.Flags().Bool("install-check", false, "Run 6-stage onboarding funnel check (used by Homebrew post-install hook)")
	RootCmd.AddCommand(doctorCmd)
}

// runDoctorCheckLegacy implements `nself doctor --check-legacy`.
// It scans global host paths for v0.9 stale artifacts (NOT per-project).
// Returns exit code 0 on clean install, non-zero with structured output on findings.
func runDoctorCheckLegacy() error {
	artifacts := migration.ScanLegacyPaths()

	if len(artifacts) == 0 {
		ui.Success("No v0.9 global artifacts detected. Clean install.")
		return nil
	}

	ui.Warn(fmt.Sprintf("Found %d v0.9 stale artifact(s) on this host:", len(artifacts)))
	fmt.Println()
	for i, a := range artifacts {
		fmt.Printf("  %d. %s\n", i+1, a.Path)
		fmt.Printf("     Kind: %s\n", a.Kind)
		fmt.Printf("     Fix:  %s\n", a.Hint)
		fmt.Println()
	}
	fmt.Println("Run the cleanup commands above, then re-run `nself doctor --check-legacy` to verify.")
	return fmt.Errorf("v0.9 stale artifacts found (%d path(s))", len(artifacts))
}

// ── Onboarding funnel check (Q08) ────────────────────────────────────────────

// installCheckJSONReport is the machine-readable shape for --install-check --json.
type installCheckJSONReport struct {
	Stages   []installCheckJSONStage `json:"stages"`
	Position int                     `json:"funnel_position"`
	Next     string                  `json:"next_action"`
}

type installCheckJSONStage struct {
	ID          int                    `json:"id"`
	Name        string                 `json:"name"`
	Status      string                 `json:"status"`
	Message     string                 `json:"message"`
	Remediation string                 `json:"remediation,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// runInstallCheck implements `nself doctor --install-check`.
//
// It runs the 6-stage onboarding funnel check, prints a friendly checklist,
// fires telemetry events for each passing stage (opt-in), and exits:
//
//	0 — all 6 stages pass
//	1 — one or more stages fail
func runInstallCheck(jsonOut bool) error {
	report := onboarding.RunFunnel()

	if jsonOut {
		return printInstallCheckJSON(report)
	}

	// ── Human-readable output ────────────────────────────────────────────────
	ui.CommandHeader("nSelf Doctor — Onboarding Funnel", "6-stage install readiness check")
	fmt.Println()

	for _, s := range report.Stages {
		label := fmt.Sprintf("Stage %d — %-12s", s.ID, s.Name)
		switch s.Status {
		case onboarding.StatusPass:
			fmt.Printf("  %s %s  %s\n", ui.C(ui.Green, ui.IconSuccess), label, s.Message)
		case onboarding.StatusFail:
			fmt.Fprintf(os.Stderr, "  %s %s  %s\n", ui.C(ui.Red, ui.IconFailure), label, s.Message)
			if s.Remediation != "" {
				fmt.Fprintf(os.Stderr, "    %s %s\n", ui.C(ui.Cyan, "→"), s.Remediation)
			}
		case onboarding.StatusUnknown:
			fmt.Fprintf(os.Stderr, "  %s %s  %s\n", ui.C(ui.Yellow, ui.IconWarning), label, s.Message)
		case onboarding.StatusSkipped:
			fmt.Printf("  %s %s  %s\n", ui.C(ui.Dim, "—"), label, s.Message)
		}
	}

	fmt.Println()
	ui.Separator()
	posStr := fmt.Sprintf("Funnel position: Stage %d/6.", report.Position)
	if report.Position == 6 {
		ui.Success(posStr + " Onboarding complete.")
	} else {
		ui.Info(posStr + " Next: " + report.Next)
	}
	fmt.Println()

	// Exit 1 if any stage failed (skipped does not count as failure for exit code).
	for _, s := range report.Stages {
		if s.Status == onboarding.StatusFail {
			return &plugin.ExitCodeError{Code: 1}
		}
	}
	return nil
}

// printInstallCheckJSON renders the funnel report as JSON.
func printInstallCheckJSON(report onboarding.FunnelReport) error {
	out := installCheckJSONReport{
		Position: report.Position,
		Next:     report.Next,
	}
	for _, s := range report.Stages {
		out.Stages = append(out.Stages, installCheckJSONStage{
			ID:          s.ID,
			Name:        s.Name,
			Status:      string(s.Status),
			Message:     s.Message,
			Remediation: s.Remediation,
			Metadata:    s.Metadata,
		})
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}
	_, err = fmt.Fprintln(os.Stdout, string(data))
	return err
}
