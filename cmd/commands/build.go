package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/migration"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Compose your infrastructure from .env",
	Long: `Generate docker-compose.yml, nginx configs, and SSL certificates
from your .env configuration.

The build pipeline:
  1. Load and validate .env cascade
  2. Generate SSL certificates (mkcert or self-signed)
  3. Generate nginx reverse-proxy configuration
  4. Generate docker-compose.yml with all enabled services`,
	RunE: runBuild,
}

func init() {
	buildCmd.Flags().BoolP("force", "f", false, "Force rebuild all components")
	buildCmd.Flags().Bool("no-cache", false, "Disable build cache")
	buildCmd.Flags().BoolP("verbose", "v", false, "Show environment cascade")
	buildCmd.Flags().Bool("debug", false, "Enable debug mode")
	buildCmd.Flags().Bool("security-report", false, "Generate security analysis")
	buildCmd.Flags().Bool("allow-insecure", false, "Allow insecure config (dev only)")
	buildCmd.Flags().Bool("check", false, "Validate only, don't build")
	buildCmd.Flags().BoolP("quiet", "q", false, "Suppress non-error output (for CI use)")
	buildCmd.Flags().Bool("no-monorepo", false, "Disable automatic monorepo backend detection")
	buildCmd.Flags().Bool("no-migration-check", false, "Skip v1 artifact detection (for automation/CI)")
	buildCmd.Flags().Bool("allow-legacy", false, "Bypass v0.9 artifact check and proceed with WARNING (not recommended)")
	buildCmd.Flags().Bool("no-auto-redis", false, "Disable automatic Redis enablement when a BullMQ-backed plugin is detected")
	buildCmd.Flags().Bool("remove-orphans", false, "Remove containers with no matching service in the freshly generated compose (G-014). Detection always runs; removal is opt-in.")
	buildCmd.Flags().Bool("hosts", false, "Opt in to /etc/hosts management for a BASE_DOMAIN that isn't a recognized local-dev domain (localhost/*.local.nself.org/*.localhost/*.local). Never overrides ENV=prod, which always skips /etc/hosts.")
	buildCmd.Flags().String("profile", "", `Service profile: curated subset of services to include in docker-compose.yml.
  app (default) — full service set, identical to pre-profile behaviour.
  ops           — observability + CI server: postgres, hasura, auth, nginx,
                  monitoring stack; excludes minio, mailpit, admin, functions, search.
Overrides NSELF_PROFILE env var. Valid values: app, ops.`)

	RootCmd.AddCommand(buildCmd)
}

func runBuild(cmd *cobra.Command, args []string) error {
	force, _ := cmd.Flags().GetBool("force")
	noCache, _ := cmd.Flags().GetBool("no-cache")
	verbose, _ := cmd.Flags().GetBool("verbose")
	debug, _ := cmd.Flags().GetBool("debug")
	securityReport, _ := cmd.Flags().GetBool("security-report")
	allowInsecure, _ := cmd.Flags().GetBool("allow-insecure")
	check, _ := cmd.Flags().GetBool("check")
	quiet, _ := cmd.Flags().GetBool("quiet")
	noMonorepo, _ := cmd.Flags().GetBool("no-monorepo")
	noMigrationCheck, _ := cmd.Flags().GetBool("no-migration-check")
	allowLegacy, _ := cmd.Flags().GetBool("allow-legacy")
	noAutoRedis, _ := cmd.Flags().GetBool("no-auto-redis")
	removeOrphans, _ := cmd.Flags().GetBool("remove-orphans")
	hosts, _ := cmd.Flags().GetBool("hosts")

	// ── Profile resolution ────────────────────────────────────────────
	// Priority: --profile flag > NSELF_PROFILE env var > default ("app").
	profileStr, _ := cmd.Flags().GetString("profile")
	if profileStr == "" {
		profileStr = os.Getenv("NSELF_PROFILE")
	}
	profile := compose.ProfileName(profileStr)
	if _, knownProfile := compose.ProfileForName(profile); !knownProfile && profileStr != "" {
		ui.Warn(fmt.Sprintf("Unknown profile %q — valid values: %s. Falling back to \"app\".", profileStr, strings.Join(compose.ValidProfiles(), ", ")))
		profile = compose.ProfileApp
	}

	if !quiet {
		ui.CommandHeader("nself build", "Generate project infrastructure")
	}

	if debug {
		_ = os.Setenv("DEBUG", "true")
	}

	if allowInsecure && !quiet {
		ui.Warn("Running with --allow-insecure: security checks relaxed")
	}

	// ── Plugin lifecycle: dormant banner + auto-remove expired plugins ───────
	// Run before the main build so users see warnings early. Auto-removal only
	// happens during build (not start) to keep start fast and non-destructive.
	runPluginLifecycleCheck(quiet)

	cwd, err := os.Getwd()
	if err != nil {
		ui.Error("Failed to determine working directory")
		return fmt.Errorf("getting working directory: %w", err)
	}

	// ── Monorepo detection ────────────────────────────────────────────────
	// Check before FindNSelfRoot so that users running nself build from the
	// monorepo root (e.g. the nself-web Turborepo) are redirected into the
	// correct backend sub-directory automatically.
	if !noMonorepo {
		if backendRoot := config.DetectMonorepoRoot(cwd); backendRoot != "" {
			if !quiet {
				fmt.Printf("→ Detected monorepo layout. Using %s as project root.\n", filepath.Base(backendRoot))
			}
			cwd = backendRoot
		}
	}

	workdir, err := config.FindNSelfRoot(cwd)
	if err != nil {
		// Before reporting "no project found", check if this is a v0.9 directory.
		if !noMigrationCheck {
			if count, names := migration.CheckLegacyProject(cwd); count >= migration.DetectionThreshold {
				if allowLegacy {
					ui.Warn(fmt.Sprintf("WARNING: v0.9 project detected (%d artifact(s): %s). Proceeding due to --allow-legacy (not recommended).", count, strings.Join(names, ", ")))
					workdir = cwd
				} else {
					ui.Error(fmt.Sprintf("v0.9 project detected. Found %d legacy artifact(s): %s", count, strings.Join(names, ", ")))
					fmt.Fprintln(os.Stderr, "Run `nself migrate` first. See https://nself.org/docs/migrate/from-v0.9")
					return fmt.Errorf("v0.9 project detected — run `nself migrate` first")
				}
			}
		}
		if workdir == "" {
			return fmt.Errorf("no nself project found in current directory or parents. Run 'nself init' to create a project")
		}
	}

	if verbose && !quiet {
		ui.Info(fmt.Sprintf("Working directory: %s", workdir))
		ui.Info(fmt.Sprintf("Force: %t | No-cache: %t | Check: %t", force, noCache, check))
	}

	// ── v0.9 artifact detection (S60-T02) ────────────────────────────────
	// Requires ≥2 of 5 heuristics to trigger (prevents false positives).
	// --no-migration-check bypasses entirely (CI/automation). --allow-legacy warns but proceeds.
	if !noMigrationCheck {
		if count, names := migration.CheckLegacyProject(workdir); count >= migration.DetectionThreshold {
			if allowLegacy {
				if !quiet {
					ui.Warn(fmt.Sprintf("WARNING: v0.9 project detected (%d artifact(s): %s). Proceeding due to --allow-legacy (not recommended).", count, strings.Join(names, ", ")))
				}
			} else {
				ui.Error(fmt.Sprintf("v0.9 project detected. Found %d legacy artifact(s): %s", count, strings.Join(names, ", ")))
				fmt.Fprintln(os.Stderr, "Run `nself migrate` first. See https://nself.org/docs/migrate/from-v0.9")
				return fmt.Errorf("v0.9 project detected — run `nself migrate` first")
			}
		} else if count == 1 && !quiet {
			ui.Warn(fmt.Sprintf("One possible v0.9 artifact found (%s). Proceeding — run `nself migrate` if this is a v0.9 project.", names[0]))
		}
	}

	// ── MLflow deprecation notice ─────────────────────────────────────────
	// MLFLOW_ENABLED=true was an optional service flag before v1.1.0. MLflow
	// is now a free plugin. Warn but continue — backward compat for 1 minor version.
	if envVals, readErr := readEnvValues(workdir + "/.env"); readErr == nil {
		if v := envVals["MLFLOW_ENABLED"]; strings.EqualFold(strings.TrimSpace(v), "true") {
			ui.Warn("DEPRECATED: MLFLOW_ENABLED is no longer an optional service. Run:")
			fmt.Fprintln(os.Stderr, "  nself plugin install mlflow")
			fmt.Fprintln(os.Stderr, "Then remove MLFLOW_ENABLED from your .env")
		}
	}

	opts := build.BuildOptions{
		Force:          force || noCache,
		Verbose:        verbose,
		Check:          check,
		SecurityReport: securityReport,
		NoAutoRedis:    noAutoRedis,
		Profile:        profile,
		Hosts:          hosts,
	}

	result, err := build.Build(workdir, opts)
	if err != nil {
		ui.Error(fmt.Sprintf("Build failed: %v", err))
		return err
	}

	if !quiet {
		if check {
			ui.Success(fmt.Sprintf("Configuration valid for project %q", result.ProjectName))
			return nil
		}

		// SSL CA trust status line.
		if result.CAInstalled {
			ui.Success("mkcert CA trusted")
		} else if result.CAManualCmd != "" {
			ui.Warn(fmt.Sprintf("Add CA manually: %s", result.CAManualCmd))
		}

		// Declared plugins that could not be wired — never silent (nself.yaml).
		if len(result.MissingPlugins) > 0 {
			ui.Warn(fmt.Sprintf("%d declared plugin(s) NOT wired into the stack: %s",
				len(result.MissingPlugins), strings.Join(result.MissingPlugins, ", ")))
			ui.Warn(fmt.Sprintf("Fix: nself plugin install %s (or remove them from nself.yaml)",
				strings.Join(result.MissingPlugins, " ")))
		}

		// /etc/hosts status line.
		if result.HostsAdded > 0 {
			ui.Success(fmt.Sprintf("Added %d domain(s) to /etc/hosts", result.HostsAdded))
		} else if result.HostsManualNote != "" {
			ui.Warn(fmt.Sprintf("Could not update /etc/hosts automatically.\n%s", result.HostsManualNote))
		}

		items := []string{
			fmt.Sprintf("Project: %s", result.ProjectName),
			fmt.Sprintf("Compose: %s", result.ComposeFile),
			fmt.Sprintf("Nginx:   %s", result.NginxConfig),
			fmt.Sprintf("SSL:     %d certificate(s)", result.SSLCerts),
			fmt.Sprintf("Files:   %d generated", result.FilesGenerated),
			fmt.Sprintf("Time:    %s", result.Duration.Round(1e6)),
		}
		ui.SummaryBox("Build Complete", items)

		ui.Info("Next step: nself start")
	}

	// ── G-014: orphan container detection (always on) + removal (opt-in
	// via --remove-orphans) — see build_orphans.go. Never runs for --check,
	// which returns before ComposeFile is generated.
	if !check && result.ComposeFile != "" {
		reportAndHandleOrphans(workdir, result, removeOrphans, quiet)
	}

	return nil
}
