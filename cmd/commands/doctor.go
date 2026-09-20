package commands

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/doctor"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// doctorCheckResult holds the outcome of a single diagnostic check.
type doctorCheckResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "pass", "warn", "fail"
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// doctorReport collects all check results for JSON output and exit code logic.
type doctorReport struct {
	Timestamp string              `json:"timestamp"`
	Checks    []doctorCheckResult `json:"checks"`
	Summary   doctorSummary       `json:"summary"`
}

type doctorSummary struct {
	Total    int `json:"total"`
	Passed   int `json:"passed"`
	Warnings int `json:"warnings"`
	Failed   int `json:"failed"`
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run comprehensive system diagnostics",
	Long: `Run comprehensive system diagnostics to validate your nSelf environment.

Checks infrastructure requirements, Docker health, disk space, memory,
network connectivity, configuration, running containers, and more.

Exit codes:
  0  All checks passed
  1  One or more checks failed
  2  Warnings only (no failures)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// ── Legacy global scan (S60-T03) ─────────────────────────────
		// --check-legacy scans for v0.9 stale paths on the host (NOT per-project).
		// Returns 0 on clean install, non-zero with structured output when found.
		checkLegacy, _ := cmd.Flags().GetBool("check-legacy")
		if checkLegacy {
			return runDoctorCheckLegacy()
		}

		// ── Onboarding funnel check (Q08) ────────────────────────────
		// --install-check runs 6-stage onboarding funnel check.
		// Invoked automatically by Homebrew post-install hook; safe to run at any time.
		installCheck, _ := cmd.Flags().GetBool("install-check")
		if installCheck {
			jsonOut2, _ := cmd.Flags().GetBool("json")
			format2, _ := cmd.Flags().GetString("format")
			if format2 == "json" {
				jsonOut2 = true
			}
			return runInstallCheck(jsonOut2)
		}

		verbose, _ := cmd.Flags().GetBool("verbose")
		full, _ := cmd.Flags().GetBool("full")
		deep, _ := cmd.Flags().GetBool("deep")
		if deep {
			full = true
		}
		// --quick was documented (help_topics.go: "Fast 10-second check") and
		// relied on by scripts/golden-path.sh's health-wait loop, but the flag
		// was never registered on this command — every invocation failed at
		// cobra's flag parser with "unknown flag: --quick" before RunE ever
		// ran, so the E2E golden-path smoke could never pass regardless of
		// actual service health. The base check set below (no --full/--deep)
		// already matches the documented "quick" behavior; --quick just makes
		// that intent explicit and wins over an accidental --full/--deep.
		quick, _ := cmd.Flags().GetBool("quick")
		if quick {
			full = false
			deep = false
		}
		fix, _ := cmd.Flags().GetBool("fix")
		jsonOut, _ := cmd.Flags().GetBool("json")
		formatFlag, _ := cmd.Flags().GetString("format")
		onlySection, _ := cmd.Flags().GetString("only")
		if formatFlag == "json" {
			jsonOut = true
		}

		// ── AI wizard mode (T-05-01) ────────────────────────────────
		aiMode, _ := cmd.Flags().GetBool("ai")
		if aiMode {
			yes, _ := cmd.Flags().GetBool("yes")
			skipOllama, _ := cmd.Flags().GetBool("skip-ollama")
			skipPool, _ := cmd.Flags().GetBool("skip-pool")
			headless, _ := cmd.Flags().GetBool("headless")
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()
			return runDoctorAI(ctx, doctorAIFlags{
				yes:        yes,
				skipOllama: skipOllama,
				skipPool:   skipPool,
				headless:   headless,
				jsonOut:    jsonOut,
			})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		cwd, _ := os.Getwd()
		// `doctor` is intentionally exempt from PersistentPreRunE's monorepo
		// chdir (see isSourceSafeCommand in root_source_guard.go — doctor must
		// stay runnable from inside the nself source repo too). But that means
		// checks below that read <projectDir>/.nself/* (e.g. OPS-DRILL-01) see
		// the invocation-time directory, while `nself backup drill` — which IS
		// redirected by that chdir — writes its log under backend/.nself/.
		// From a monorepo root the two therefore disagree, and OPS-DRILL-01
		// reports "no drill recorded" even right after a real one ran. Detect
		// the same layout here, scoped to doctor's own projectDir variable, so
		// its reads land where backup's writes did without re-enabling the
		// source-repo guard doctor is deliberately exempt from.
		if backendRoot := config.DetectMonorepoRoot(cwd); backendRoot != "" {
			cwd = backendRoot
		}

		// Deep mode: run all 12 subsystem checks via doctor package
		if deep {
			deepResults := doctor.DeepChecks(ctx, cwd, verbose)

			// Apply --only filter
			if onlySection != "" {
				deepResults = doctor.FilterBySection(deepResults, onlySection)
			}

			// Apply fix-it engine
			if fix {
				doctor.FixItEngine(ctx, deepResults)
			}

			// Convert to doctorCheckResult
			var checks []doctorCheckResult
			for _, r := range deepResults {
				checks = append(checks, doctorCheckResult{
					Name:    fmt.Sprintf("[%s] %s", r.Section, r.Name),
					Status:  r.Status,
					Message: r.Message,
					Detail:  r.FixCmd,
				})
			}

			report := buildDoctorReport(checks)
			if jsonOut {
				return printDoctorJSON(report)
			}
			if !jsonOut {
				ui.CommandHeader("nSelf Doctor (Deep)", "All 12 subsystem checks")
			}
			printDoctorSummary(report)
			if report.Summary.Failed > 0 {
				return &plugin.ExitCodeError{Code: 1}
			}
			if report.Summary.Warnings > 0 {
				return &plugin.ExitCodeError{Code: 2}
			}
			return nil
		}

		if !jsonOut {
			ui.CommandHeader("nSelf Doctor", "System diagnostics")
		}

		var checks []doctorCheckResult

		// 1. Infrastructure checks
		if !jsonOut {
			ui.Section("Infrastructure")
		}
		checks = append(checks, checkDockerInstalled(verbose))
		checks = append(checks, checkDockerRunning(ctx, verbose))
		checks = append(checks, checkDockerComposeVersion(ctx, verbose))
		checks = append(checks, checkGitInstalled(verbose))

		// 2. Disk space
		if !jsonOut {
			ui.Section("Disk Space")
		}
		checks = append(checks, checkDiskSpace(verbose))

		// 3. Memory (full mode only — slower)
		if full {
			if !jsonOut {
				ui.Section("Memory")
			}
			checks = append(checks, checkMemory(verbose))
		}

		// 4. Network (full mode only — slower)
		if full {
			if !jsonOut {
				ui.Section("Network")
			}
			checks = append(checks, checkNetwork(ctx, verbose))
		}

		// 5. Port availability
		if !jsonOut {
			ui.Section("Port Availability")
		}
		checks = append(checks, checkPorts(ctx, cwd, verbose)...)
		checks = append(checks, checkServicePortConflicts(ctx, cwd, verbose)...)
		checks = append(checks, checkHomebrewPostgres(verbose)...)

		// 6. Configuration
		if !jsonOut {
			ui.Section("Configuration")
		}
		checks = append(checks, checkEnvExists(cwd, verbose))
		checks = append(checks, checkPasswordStrength(cwd, verbose, fix)...)
		checks = append(checks, checkJWTSecretPresent(cwd, verbose))

		// 7. Route consistency + port range sanity + config validators
		if !jsonOut {
			ui.Section("Config Validation")
		}
		checks = append(checks, checkRouteConsistency(cwd, verbose)...)
		checks = append(checks, checkPortRangeSanity(cwd, verbose)...)
		checks = append(checks, checkConfigValidators(cwd, verbose))

		// 8. Plugin compatibility
		if !jsonOut {
			ui.Section("Plugins")
		}
		checks = append(checks, checkPluginCompatibility(cwd, verbose)...)

		// 9. License cache state
		if !jsonOut {
			ui.Section("License")
		}
		checks = append(checks, checkLicenseCache(verbose))
		checks = append(checks, checkLicenseMigrationRate(verbose))

		// 9. Container health
		if !jsonOut {
			ui.Section("Container Health")
		}
		checks = append(checks, checkContainerHealth(ctx, cwd, verbose)...)

		// Build summary
		report := buildDoctorReport(checks)

		if jsonOut {
			return printDoctorJSON(report)
		}

		// Print summary
		printDoctorSummary(report)

		// Exit code: 1=failures, 2=warnings only, 0=all pass
		if report.Summary.Failed > 0 {
			return &plugin.ExitCodeError{Code: 1}
		}
		if report.Summary.Warnings > 0 {
			return &plugin.ExitCodeError{Code: 2}
		}
		return nil
	},
}
